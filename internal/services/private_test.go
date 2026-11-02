package services

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// privateEnv wires a Mode to a settings file as main does.
func privateEnv(t *testing.T, o privatemode.Options) (*PrivateService, *config.Store, *recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := config.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(*config.Settings) {}); err != nil { // write the file
		t.Fatal(err)
	}
	o.Saved = st.Get().Private.Enabled
	o.Save = func(on bool) error {
		_, err := st.Update(func(s *config.Settings) { s.Private.Enabled = on })
		return err
	}
	rec := &recorder{}
	e := &Env{Deps: Deps{Settings: st, Emit: rec, Logger: discard(), Private: privatemode.New(o)}}
	t.Cleanup(e.close)
	return newPrivateService(e), st, rec, path
}

func TestPrivateSetSavesTheSetting(t *testing.T) {
	s, st, rec, _ := privateEnv(t, privatemode.Options{})
	got, err := s.Set(true)
	if err != nil || got != (PrivateState{On: true, Source: "setting", Saved: true}) {
		t.Fatalf("Set(true) = %+v, %v", got, err)
	}
	if !st.Get().Private.Enabled {
		t.Error("setting not saved")
	}
	// The frontend hears the new state and the settings it must not
	// overwrite with a stale copy.
	if !slices.Contains(rec.events, EventPrivateChanged) || !slices.Contains(rec.events, EventSettingsChanged) {
		t.Errorf("events %v", rec.events)
	}

	// A change of the setting elsewhere (Settings, an import) applies.
	if _, err := st.Update(func(c *config.Settings) { c.Private.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	if s.State().On {
		t.Error("setting turned off elsewhere, still on")
	}
}

// A development build starts private with the setting off and toggles this
// session only: settings.json is byte-for-byte unchanged.
func TestPrivateDevelopmentBuildLeavesTheSettingAlone(t *testing.T) {
	s, st, rec, path := privateEnv(t, privatemode.Options{Dev: true})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.State(); got != (PrivateState{On: true, Source: "development", Saved: false}) {
		t.Fatalf("state = %+v", got)
	}
	if got, err := s.Set(false); err != nil || got.On {
		t.Fatalf("Set(false) = %+v, %v", got, err)
	}
	if got, err := s.Set(true); err != nil || !got.On {
		t.Fatalf("Set(true) = %+v, %v", got, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || st.Get().Private.Enabled {
		t.Errorf("settings changed:\n%s\n→\n%s", before, after)
	}
	if slices.Contains(rec.events, EventSettingsChanged) {
		t.Errorf("settings:changed sent for a session toggle: %v", rec.events)
	}
}

func TestPrivateForcedCannotBeTurnedOff(t *testing.T) {
	s, _, _, _ := privateEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	got, err := s.Set(false)
	if CodeOf(err) != CodePrivateForced || !got.On || got.Source != "environment" {
		t.Errorf("Set(false) = %+v, %v (code %s)", got, err, CodeOf(err))
	}
}
