package services

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

func systemEnv(t *testing.T, prepare func(*config.Settings)) (*SystemService, *config.Store, *recorder) {
	t.Helper()
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if prepare != nil {
		if _, err := st.Update(prepare); err != nil {
			t.Fatal(err)
		}
	}
	rec := &recorder{}
	e := &Env{Deps: Deps{Settings: st, Emit: rec, Logger: discard()}}
	t.Cleanup(e.close)
	return &SystemService{o: e}, st, rec
}

func TestSettingsExportImportBetweenComputers(t *testing.T) {
	from, _, _ := systemEnv(t, func(s *config.Settings) {
		s.Theme = config.ThemeDark
		s.Desktop.CloseAction = config.CloseQuits
		s.Window = config.Window{Width: 2000, Height: 1200}
		s.Audio.OutputDevice = "alsa_output.usb-dac"
	})
	file := filepath.Join(t.TempDir(), "sona-settings.json")
	if err := from.exportSettings(file); err != nil {
		t.Fatal(err)
	}

	to, st, rec := systemEnv(t, nil)
	var observed []string
	st.OnChange(func(prev, next config.Settings) { observed = append(observed, next.Theme) })
	got, err := to.importSettings(file)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"desktop.closeAction", "theme"}; got.Path != file || !slices.Equal(got.Changed, want) {
		t.Fatalf("import = %+v, want changed %v", got, want)
	}
	cur := st.Get()
	if cur.Theme != config.ThemeDark || cur.Window != config.Defaults().Window || cur.Audio.OutputDevice != "" {
		t.Fatalf("settings after import: %+v", cur)
	}
	if !slices.Equal(observed, []string{config.ThemeDark}) {
		t.Fatalf("observers saw %v", observed)
	}
	if !slices.Equal(rec.events, []string{EventSettingsChanged}) {
		t.Fatalf("events = %v", rec.events)
	}

	// The same file again changes nothing and tells nobody.
	again, err := to.importSettings(file)
	if err != nil || len(again.Changed) != 0 || len(rec.events) != 1 {
		t.Fatalf("re-import = %+v, %v; events %v", again, err, rec.events)
	}
}

func TestSettingsImportRejectsOtherFilesUnchanged(t *testing.T) {
	sys, st, rec := systemEnv(t, nil)
	runs := 0
	st.OnChange(func(prev, next config.Settings) { runs++ })
	dir := t.TempDir()
	for name, body := range map[string]string{
		"settings.json": `{"theme": "dark"}`,
		"newer.json":    `{"format": "sona-settings", "version": 99, "settings": {"theme": "dark"}}`,
		"typed.json":    `{"format": "sona-settings", "version": 1, "settings": {"theme": "dark", "volume": "loud", "audioQuality": "high"}}`,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := sys.importSettings(path); err == nil {
			t.Errorf("%s: imported", name)
		}
	}
	if _, err := sys.importSettings(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("missing file imported")
	}
	if st.Get().Theme != config.ThemeSystem || runs != 0 || len(rec.events) != 0 {
		t.Fatalf("a rejected file changed something: theme %s, %d observer runs, events %v", st.Get().Theme, runs, rec.events)
	}
}

// savedVolume waits up to five seconds for the settings to hold want
// (the player saves its volume a second after the last change).
func savedVolume(t *testing.T, st *config.Store, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for st.Get().Volume != want {
		if time.Now().After(deadline) {
			t.Fatalf("saved volume = %v, want %v", st.Get().Volume, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A volume set from outside Sona's window (MPRIS: the desktop's media
// controls) is saved like the player bar's, without waiting for quit.
func TestVolumeFromMPRISIsSaved(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	mp := NewMPRISPlayer(ce.e.Coord, func() {}, func() {}, discard())
	if err := mp.SetVolume(t.Context(), 0.42); err != nil {
		t.Fatal(err)
	}
	savedVolume(t, ce.settings, 0.42)
}

// Settings saved from a page that read them before the player's volume
// changed keep the player's volume: the volume has one writer.
func TestUpdateSettingsKeepsThePlayersVolume(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{})
	stale := ce.settings.Get()
	if err := ce.play.SetVolume(t.Context(), 0.3); err != nil {
		t.Fatal(err)
	}
	savedVolume(t, ce.settings, 0.3)

	stale.ReduceEffects = true
	saved, err := (&SystemService{o: ce.e}).UpdateSettings(stale)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.ReduceEffects || saved.Volume != 0.3 {
		t.Errorf("saved %+v, want reduce effects on and the player's volume 0.3", saved)
	}
	if v := ce.settings.Get().Volume; v != 0.3 {
		t.Errorf("stored volume = %v", v)
	}
	if v := ce.e.Coord.Snapshot().Volume; v != 0.3 {
		t.Errorf("playing volume = %v", v)
	}
}
