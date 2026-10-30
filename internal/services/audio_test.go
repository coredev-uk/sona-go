package services

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// audioEng is a playEngine that records output settings and, like the
// native engine, turns the volume into playback.VolumeGain for its output.
type audioEng struct {
	playEngine
	mu      sync.Mutex
	gains   []float64
	eqs     []cdmhost.Equalizer
	devices []string
	checks  []bool
}

func (e *audioEng) SetVolume(_ context.Context, v float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.gains = append(e.gains, playback.VolumeGain(v))
	return nil
}

func (e *audioEng) SetEqualizer(_ context.Context, eq cdmhost.Equalizer) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eqs = append(e.eqs, eq)
	return nil
}

func (e *audioEng) SetOutputDevice(_ context.Context, d string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.devices = append(e.devices, d)
	return nil
}
func (e *audioEng) SetCrossfade(time.Duration) {}
func (e *audioEng) SetSoundCheck(_ context.Context, on bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.checks = append(e.checks, on)
	return nil
}

// MPRIS Volume is the slider value both ways; the engine alone applies
// the cubic curve to it.
func TestMPRISVolumeIsTheSliderValue(t *testing.T) {
	eng := &audioEng{playEngine: playEngine{events: make(chan playback.Event)}}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	p := NewMPRISPlayer(coord, func() {}, func() {}, discard())
	if err := p.SetVolume(t.Context(), 0.5); err != nil {
		t.Fatal(err)
	}
	if v := p.Snapshot().Volume; v != 0.5 {
		t.Errorf("MPRIS Volume reads %v after setting 0.5", v)
	}
	eng.mu.Lock()
	defer eng.mu.Unlock()
	if len(eng.gains) != 1 || eng.gains[0] != 0.125 {
		t.Errorf("output gains %v, want [0.125] (0.5³)", eng.gains)
	}
}

// The saved equaliser, device and Sound Check reach the engine when the
// app starts (a restart), and changes reach it as they are saved.
func TestAudioSettingsApplyAtStartAndOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	first, err := config.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Update(func(s *config.Settings) {
		s.Audio.Equalizer.Enabled, s.Audio.Equalizer.Preset, s.Audio.Equalizer.Preamp = true, "bass-boost", -2
		s.Audio.OutputDevice, s.Audio.SoundCheck = "alsa_output.usb", true
	}); err != nil {
		t.Fatal(err)
	}

	settings, err := config.OpenStore(path) // the next start
	if err != nil {
		t.Fatal(err)
	}
	eng := &audioEng{}
	newAudioService(&Env{Deps: Deps{Settings: settings, Engine: eng, Logger: discard()}})
	want := cdmhost.Equalizer{Pregain: -8, Gains: [10]float64{6, 5, 4, 2.5, 1}}
	if len(eng.eqs) != 1 || eng.eqs[0] != want || len(eng.devices) != 1 || eng.devices[0] != "alsa_output.usb" ||
		len(eng.checks) != 1 || !eng.checks[0] {
		t.Fatalf("at start: eqs %v devices %q sound check %v", eng.eqs, eng.devices, eng.checks)
	}

	if _, err := settings.Update(func(s *config.Settings) { s.Audio.Equalizer.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Update(func(s *config.Settings) { s.Theme = config.ThemeDark }); err != nil {
		t.Fatal(err)
	}
	if len(eng.eqs) != 2 || eng.eqs[1] != (cdmhost.Equalizer{}) || len(eng.devices) != 1 || len(eng.checks) != 1 {
		t.Errorf("after turning the equaliser off and an unrelated change: eqs %v devices %q checks %v", eng.eqs, eng.devices, eng.checks)
	}
}
