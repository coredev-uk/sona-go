package privatemode

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestEnv(t *testing.T) {
	for _, tc := range []struct {
		v       string
		want    Override
		wantErr bool
	}{
		{"", OverrideNone, false},
		{"1", OverrideOn, false},
		{" true ", OverrideOn, false},
		{"ON", OverrideOn, false},
		{"0", OverrideOff, false},
		{"false", OverrideOff, false},
		{"off", OverrideOff, false},
		// A typo never reports plays: it forces private mode on.
		{"2", OverrideOn, true},
		{"flase", OverrideOn, true},
	} {
		got, err := Env(tc.v)
		if got != tc.want || (err != nil) != tc.wantErr {
			t.Errorf("Env(%q) = %v, %v; want %v, error %v", tc.v, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestSources(t *testing.T) {
	for _, tc := range []struct {
		name string
		o    Options
		want State
	}{
		{"release, setting off", Options{}, State{false, SourceSetting}},
		{"release, setting on", Options{Saved: true}, State{true, SourceSetting}},
		{"development, setting off", Options{Dev: true}, State{true, SourceDevelopment}},
		{"development, SONA_PRIVATE=0", Options{Dev: true, Override: OverrideOff}, State{false, SourceSetting}},
		{"development, SONA_PRIVATE=0, setting on", Options{Dev: true, Saved: true, Override: OverrideOff}, State{true, SourceSetting}},
		{"release, SONA_PRIVATE=1", Options{Override: OverrideOn}, State{true, SourceEnvironment}},
		{"release, SONA_PRIVATE=0", Options{Override: OverrideOff}, State{false, SourceSetting}},
	} {
		if got := New(tc.o).State(); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// saver records the setting Set writes, as the settings store would.
type saver struct {
	saved []bool
	err   error
}

func (s *saver) save(on bool) error {
	if s.err != nil {
		return s.err
	}
	s.saved = append(s.saved, on)
	return nil
}

func TestSetWritesTheSettingOnlyFromTheSetting(t *testing.T) {
	var s saver
	m := New(Options{Save: s.save})
	if st, err := m.Set(true); err != nil || !st.On {
		t.Fatalf("Set(true) = %+v, %v", st, err)
	}
	if st, err := m.Toggle(); err != nil || st.On {
		t.Fatalf("Toggle = %+v, %v", st, err)
	}
	if len(s.saved) != 2 || !s.saved[0] || s.saved[1] {
		t.Errorf("saved %v, want [true false]", s.saved)
	}
	if m.Saved() {
		t.Error("Saved() after turning off")
	}

	// A save that fails leaves the state as it was.
	s.err = errors.New("disk full")
	if st, err := m.Set(true); err == nil || st.On {
		t.Errorf("failed save: %+v, %v", st, err)
	}
}

// A development build toggles this session only: the saved setting is
// never written, and a setting changed elsewhere does not override it.
func TestDevelopmentBuildIsSessionOnly(t *testing.T) {
	var s saver
	m := New(Options{Dev: true, Save: s.save})
	if st, err := m.Set(false); err != nil || st != (State{false, SourceDevelopment}) {
		t.Fatalf("Set(false) = %+v, %v", st, err)
	}
	if st, _ := m.Toggle(); !st.On {
		t.Fatal("Toggle did not turn it back on")
	}
	m.SavedChanged(false)
	if !m.On() {
		t.Error("a saved-setting change overrode the session state")
	}
	if len(s.saved) != 0 {
		t.Errorf("the saved setting was written: %v", s.saved)
	}
}

func TestEnvironmentForcesOn(t *testing.T) {
	var s saver
	m := New(Options{Override: OverrideOn, Save: s.save})
	if _, err := m.Set(false); !errors.Is(err, ErrForced) {
		t.Fatalf("Set(false) err = %v, want ErrForced", err)
	}
	if _, err := m.Toggle(); !errors.Is(err, ErrForced) {
		t.Fatalf("Toggle err = %v, want ErrForced", err)
	}
	if st, err := m.Set(true); err != nil || !st.On {
		t.Fatalf("Set(true) = %+v, %v", st, err)
	}
	m.SavedChanged(false)
	if !m.On() || len(s.saved) != 0 {
		t.Errorf("on %v, saved %v", m.On(), s.saved)
	}
}

func TestSavedChangedFollowsTheSetting(t *testing.T) {
	m := New(Options{})
	var seen []State
	m.OnChange(func(st State) { seen = append(seen, st) })
	m.SavedChanged(true)
	m.SavedChanged(true) // no change, no notification
	m.SavedChanged(false)
	want := []State{{true, SourceSetting}, {false, SourceSetting}}
	if len(seen) != len(want) || seen[0] != want[0] || seen[1] != want[1] {
		t.Errorf("changes %+v, want %+v", seen, want)
	}
}

// Since: a listen is private if private mode is on now or was on at any
// point after it started.
func TestSince(t *testing.T) {
	now := time.Unix(1000, 0)
	m := New(Options{Now: func() time.Time { return now }})
	before := now.Add(-time.Minute)
	if m.Since(before) {
		t.Fatal("never on, but private")
	}
	now = now.Add(time.Minute)
	_, _ = m.Set(true)
	if !m.Since(now) {
		t.Fatal("on, but not private")
	}
	now = now.Add(time.Minute)
	_, _ = m.Set(false)
	if !m.Since(before) {
		t.Error("a listen that was private at some point became public")
	}
	now = now.Add(time.Minute)
	if m.Since(now) {
		t.Error("a listen started after private mode turned off is private")
	}
	var nilMode *Mode
	if nilMode.Since(before) {
		t.Error("nil Mode is private")
	}
}

func TestCommand(t *testing.T) {
	var s saver
	m := New(Options{Save: s.save})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "private listening: off"},
		{[]string{"status"}, "private listening: off"},
		{[]string{"on"}, "private listening: on"},
		{[]string{"toggle"}, "private listening: off"},
		{[]string{"toggle"}, "private listening: on"},
		{[]string{"off"}, "private listening: off"},
	} {
		got, err := m.Command(tc.args)
		if err != nil || got != tc.want {
			t.Errorf("Command(%q) = %q, %v; want %q", tc.args, got, err, tc.want)
		}
	}
	for _, bad := range [][]string{{"maybe"}, {"on", "now"}} {
		if _, err := m.Command(bad); err == nil || !strings.HasPrefix(err.Error(), "usage:") {
			t.Errorf("Command(%q) err = %v, want usage", bad, err)
		}
	}

	dev := New(Options{Dev: true})
	if got, _ := dev.Command(nil); got != "private listening: on (development build: this session only; saved setting off)" {
		t.Errorf("development status = %q", got)
	}
	forced := New(Options{Override: OverrideOn})
	if got, _ := forced.Command(nil); got != "private listening: on (forced by SONA_PRIVATE=1)" {
		t.Errorf("forced status = %q", got)
	}
	if _, err := forced.Command([]string{"off"}); !errors.Is(err, ErrForced) {
		t.Errorf("forced off: err = %v", err)
	}
}
