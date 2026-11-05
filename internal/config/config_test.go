package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	s, err := LoadSettings(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, Defaults()) {
		t.Fatalf("got %+v, want defaults", s)
	}
	d := Defaults()
	if d.Theme != ThemeSystem || d.AudioQuality != Quality256 || d.Volume <= 0 || d.EngineIdleMinutes <= 0 ||
		d.ReduceEffects {
		t.Fatalf("unexpected defaults %+v", d)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// From the defaults: zero nested settings normalize to their defaults.
	in := Defaults()
	in.Theme = ThemeDark
	in.ReduceEffects = true
	in.Volume = 0.35
	in.AudioQuality = Quality64
	in.EngineIdleMinutes = 3
	in.Shortcuts = Shortcuts{"player.toggle": "K"}
	in.Desktop = Desktop{CloseAction: CloseQuits, Autostart: true, StartPage: "/radio", ResumeLastRoute: true, LastRoute: "/album/1?song=2"}
	in.Window = Window{Width: 1600, Height: 1000, Maximised: true}
	if err := SaveSettings(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip: got %+v, want %+v", out, in)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestLoadPartialKeepsDefaultsAndNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"neon","volume":4,"audioQuality":128,"futureKey":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.Volume = 1
	if !reflect.DeepEqual(s, want) {
		t.Fatalf("got %+v, want %+v", s, want)
	}
}

func TestLoadMalformedReturnsDefaultsAndError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(path)
	if err == nil {
		t.Fatal("want parse error")
	}
	if !reflect.DeepEqual(s, Defaults()) {
		t.Fatalf("got %+v, want defaults", s)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	old := Defaults()
	old.Theme = ThemeLight
	if err := SaveSettings(path, old); err != nil {
		t.Fatal(err)
	}
	next := old
	next.Theme = ThemeDark
	if err := SaveSettings(path, next); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" {
		t.Fatalf("temp files left behind: %v", entries)
	}

	// A failing replace (the target is a non-empty directory) must leave no
	// temp file and must not touch the directory's contents.
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "keep"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := SaveSettings(blocked, next); err == nil {
		t.Fatal("want error replacing a directory")
	}
	entries, _ = os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file left after failed save: %s", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(blocked, "keep")); err != nil {
		t.Fatalf("failed save disturbed target: %v", err)
	}
	got, err := LoadSettings(path)
	if err != nil || got.Theme != ThemeDark {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestStoreUpdatePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := st.Update(func(s *Settings) { s.ReduceEffects = true; s.Volume = -1 })
	if err != nil {
		t.Fatal(err)
	}
	if !got.ReduceEffects || got.Volume != 0 || !reflect.DeepEqual(st.Get(), got) {
		t.Fatalf("update: got %+v, store %+v", got, st.Get())
	}
	reread, err := LoadSettings(path)
	if err != nil || !reflect.DeepEqual(reread, got) {
		t.Fatalf("persisted %+v, %v; want %+v", reread, err, got)
	}
}

func TestStoreUpdateKeepsStateOnSaveError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-dir", "settings.json")
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *Settings) { s.Theme = ThemeDark }); err == nil {
		t.Fatal("want save error")
	}
	if !reflect.DeepEqual(st.Get(), Defaults()) {
		t.Fatalf("in-memory settings changed after failed save: %+v", st.Get())
	}
}

// An Update that writes into the equaliser curve in place must not change
// the previous settings observers compare against.
func TestStoreUpdateObserversSeeTheOldCurve(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var prevGain, nextGain float64
	st.OnChange(func(prev, next Settings) {
		prevGain, nextGain = prev.Audio.Equalizer.Custom[0], next.Audio.Equalizer.Custom[0]
	})
	if _, err := st.Update(func(s *Settings) { s.Audio.Equalizer.Custom[0] = 6 }); err != nil {
		t.Fatal(err)
	}
	if prevGain != 0 || nextGain != 6 {
		t.Fatalf("observer saw prev %v, next %v; want 0, 6", prevGain, nextGain)
	}
	got := st.Get()
	got.Audio.Equalizer.Custom[0] = -3
	if st.Get().Audio.Equalizer.Custom[0] != 6 {
		t.Fatal("editing a Get copy changed the store")
	}
}

// UpdateThen's callback is what keeps announcements in save order: it runs
// after the observers and while the store still refuses another Update, so
// a save an observer hands to another goroutine cannot be announced first.
func TestStoreUpdateThenRunsAfterObserversBeforeTheNextUpdate(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var steps []string
	st.OnChange(func(prev, next Settings) { steps = append(steps, "observer") })
	ran := false
	saved, err := st.UpdateThen(func(s *Settings) { s.Theme = ThemeDark }, func(prev, saved Settings) {
		ran = true
		steps = append(steps, "then")
		if prev.Theme == ThemeDark || saved.Theme != ThemeDark {
			t.Errorf("then saw theme %q -> %q", prev.Theme, saved.Theme)
		}
		if st.obsMu.TryLock() {
			st.obsMu.Unlock()
			t.Error("another Update could start before then returned")
		}
	})
	if err != nil || !ran || saved.Theme != ThemeDark {
		t.Fatalf("UpdateThen = %+v, %v (then ran: %v)", saved, err, ran)
	}
	if !slices.Equal(steps, []string{"observer", "then"}) {
		t.Fatalf("order = %v", steps)
	}
}

// The save fails whoever runs the test (root writes into a read-only
// directory, so the CI image's root would pass a chmod): the directory the
// settings live in is replaced by a file, and the temporary file can't be
// created.
func TestStoreUpdateThenSkipsThenOnASaveError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sona")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := OpenStore(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = st.UpdateThen(func(s *Settings) { s.Theme = ThemeDark }, func(Settings, Settings) {
		t.Error("then ran after a failed save")
	})
	if err == nil {
		t.Fatal("UpdateThen saved into a directory that is a file")
	}
}

func TestEngineIdleMinutesBounds(t *testing.T) {
	for in, want := range map[int]int{-5: 10, 0: 10, 30: 30, 1 << 40: 24 * 60} {
		s := Defaults()
		s.EngineIdleMinutes = in
		if got := s.Normalized().EngineIdleMinutes; got != want {
			t.Errorf("Normalized(%d) = %d, want %d", in, got, want)
		}
	}
}

// A null or partly blank shortcuts object loads as an object holding only
// real overrides: the frontend reads it without a nil check, and a blank
// binding means "use the default", not "no key".
func TestLoadShortcutsNullAndBlank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	for _, body := range []string{`{"shortcuts":null}`, `{"shortcuts":{"player.toggle":" ","":"Ctrl+K"," commands.palette ":" Ctrl+P "}}`} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		s, err := LoadSettings(path)
		if err != nil {
			t.Fatal(err)
		}
		want := Shortcuts{}
		if strings.Contains(body, "palette") {
			want = Shortcuts{"commands.palette": "Ctrl+P"}
		}
		if s.Shortcuts == nil || !reflect.DeepEqual(s.Shortcuts, want) {
			t.Errorf("%s: shortcuts = %#v, want %#v", body, s.Shortcuts, want)
		}
	}
}

func TestStorageArtCacheLimit(t *testing.T) {
	for in, want := range map[int]int{0: DefaultArtCacheMiB, 1: MinArtCacheMiB, -5: MinArtCacheMiB, 2048: 2048, 1 << 20: MaxArtCacheMiB} {
		if got := (Storage{ArtCacheMiB: in}).normalized().ArtCacheMiB; got != want {
			t.Errorf("artCacheMiB %d → %d, want %d", in, got, want)
		}
	}
}

func TestCrossfadeSecondsBounds(t *testing.T) {
	for in, want := range map[int]int{0: 6, -3: 1, 1: 1, 12: 12, 30: 12} {
		if got := (Settings{Crossfade: Crossfade{Seconds: in}}).Normalized().Crossfade.Seconds; got != want {
			t.Errorf("crossfade %d s → %d, want %d", in, got, want)
		}
	}
	if d := Defaults().Crossfade; d.Enabled || d.Seconds != 6 {
		t.Errorf("default crossfade %+v, want off at 6 s", d)
	}
}

func TestLanguageKeepsWellFormedTags(t *testing.T) {
	for in, want := range map[string]string{
		"":           LanguageAutomatic,
		"en":         "en",
		"fr-CA":      "fr-CA",
		"zh-Hant-TW": "zh-Hant-TW",
		"es-419":     "es-419",
		"fr_CA":      LanguageAutomatic,
		"../etc":     LanguageAutomatic,
		"automatic":  LanguageAutomatic,
		" fr":        LanguageAutomatic,
	} {
		if got := (Settings{Language: in}).Normalized().Language; got != want {
			t.Errorf("language %q → %q, want %q", in, got, want)
		}
	}
	if Defaults().Language != LanguageAutomatic {
		t.Errorf("default language %q, want Automatic", Defaults().Language)
	}
}
