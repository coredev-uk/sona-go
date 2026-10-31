package services

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
)

// audioEngine is the output processing an engine offers (native.Engine).
type audioEngine interface {
	SetEqualizer(ctx context.Context, eq equalizer.Curve) error
	SetOutputDevice(ctx context.Context, device string) error
	SetSoundCheck(ctx context.Context, on bool) error
	SetCrossfade(d time.Duration)
	OutputDevices() ([]playback.OutputDevice, error)
}

// AudioService is the equaliser, Sound Check and the output device, and
// the crossfade between tracks. The settings (config.Audio,
// config.Crossfade, saved through SystemService.UpdateSettings) are
// applied to the engine at start and whenever they change.
type AudioService struct {
	o   *Env
	eng audioEngine // nil when the engine has no output processing
}

func newAudioService(e *Env) *AudioService {
	s := &AudioService{o: e}
	s.eng, _ = e.Engine.(audioEngine)
	if s.eng == nil {
		return s
	}
	cur := e.Settings.Get().Audio
	ctx := context.Background()
	s.apply("equalizer", s.eng.SetEqualizer(ctx, hostEqualizer(cur.Equalizer)))
	s.apply("output device", s.eng.SetOutputDevice(ctx, cur.OutputDevice))
	s.apply("sound check", s.eng.SetSoundCheck(ctx, cur.SoundCheck))
	s.eng.SetCrossfade(crossfade(e.Settings.Get().Crossfade))
	e.Settings.OnChange(func(prev, next config.Settings) {
		if prev.Crossfade != next.Crossfade {
			s.eng.SetCrossfade(crossfade(next.Crossfade))
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p, n := prev.Audio, next.Audio
		if !p.Equalizer.Equal(n.Equalizer) {
			s.apply("equalizer", s.eng.SetEqualizer(ctx, hostEqualizer(n.Equalizer)))
		}
		if p.OutputDevice != n.OutputDevice {
			s.apply("output device", s.eng.SetOutputDevice(ctx, n.OutputDevice))
		}
		if p.SoundCheck != n.SoundCheck {
			s.apply("sound check", s.eng.SetSoundCheck(ctx, n.SoundCheck))
		}
	})
	return s
}

func (s *AudioService) apply(what string, err error) {
	if err != nil {
		s.o.Logger.Warn("audio: apply "+what, "err", err)
	}
}

// crossfade is the engine's crossfade for the setting; 0 plays gapless.
func crossfade(c config.Crossfade) time.Duration {
	if !c.Enabled {
		return 0
	}
	return time.Duration(c.Seconds) * time.Second
}

// hostEqualizer turns the setting into what the playback host applies.
func hostEqualizer(e config.Equalizer) equalizer.Curve {
	return equalizer.Curve{Pregain: e.Pregain(), Gains: e.Gains()}
}

// Equalizer describes the equaliser's bands, limits and presets.
func (s *AudioService) Equalizer() EqualizerInfo {
	out := EqualizerInfo{
		Frequencies: equalizer.Frequencies[:],
		MinGain:     equalizer.MinGain, MaxGain: equalizer.MaxGain,
		MinPreamp: equalizer.MinPreamp, MaxPreamp: equalizer.MaxPreamp,
		Presets: make([]EqualizerPreset, len(equalizer.Presets)),
	}
	for i, p := range equalizer.Presets {
		out.Presets[i] = EqualizerPreset{ID: p.ID, Name: p.Name, Gains: append([]float64(nil), p.Gains[:]...)}
	}
	return out
}

// PreviewEqualizer applies eq to the playing audio without saving it, for
// live feedback while a control is dragged; save the final value with
// SystemService.UpdateSettings.
func (s *AudioService) PreviewEqualizer(ctx context.Context, eq config.Equalizer) error {
	if s.eng == nil {
		return nil
	}
	return s.eng.SetEqualizer(ctx, hostEqualizer(eq))
}

// OutputDevices lists the audio outputs (PipeWire sinks). The list is
// empty when PipeWire's GStreamer plugin is missing; playback then always
// follows the default output. Without an engine that chooses its output
// it fails with CodeUnsupported.
func (s *AudioService) OutputDevices() ([]OutputDevice, error) {
	if s.eng == nil {
		return []OutputDevice{}, unsupported("choosing the output device")
	}
	devs, err := s.eng.OutputDevices()
	if err != nil {
		return []OutputDevice{}, err
	}
	out := make([]OutputDevice, len(devs))
	for i, d := range devs {
		out[i] = OutputDevice{Name: d.Name, Description: d.Description, Default: d.Default}
	}
	return out, nil
}
