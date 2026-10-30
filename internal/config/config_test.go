package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolvePathsXDG(t *testing.T) {
	p, err := ResolvePaths(envMap(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "/x/config",
		"XDG_DATA_HOME":   "/x/data",
		"XDG_CACHE_HOME":  "/x/cache",
		"XDG_STATE_HOME":  "/x/state",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Config: "/x/config/sona",
		Data:   "/x/data/sona",
		Cache:  "/x/cache/sona",
		Art:    "/x/cache/sona/art",
		State:  "/x/state/sona",
	}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestResolvePathsDefaultsAndRelativeIgnored(t *testing.T) {
	p, err := ResolvePaths(envMap(map[string]string{
		"HOME":            "/home/u",
		"XDG_CONFIG_HOME": "relative/config", // ignored per the XDG spec
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := Paths{
		Config: "/home/u/.config/sona",
		Data:   "/home/u/.local/share/sona",
		Cache:  "/home/u/.cache/sona",
		Art:    "/home/u/.cache/sona/art",
		State:  "/home/u/.local/state/sona",
	}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestResolvePathsNoHome(t *testing.T) {
	if _, err := ResolvePaths(envMap(nil)); err == nil {
		t.Fatal("want error without HOME or XDG variables")
	}
}

func TestEnsureCreatesAndTightens(t *testing.T) {
	root := t.TempDir()
	p, err := ResolvePaths(envMap(map[string]string{"HOME": root}))
	if err != nil {
		t.Fatal(err)
	}
	// A pre-existing, world-readable config dir must be tightened.
	if err := os.MkdirAll(p.Config, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{p.Config, p.Data, p.Cache, p.Art, p.State} {
		fi, err := os.Stat(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.IsDir() || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, want dir 0700", dir, fi.Mode())
		}
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	s, err := LoadSettings(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if s != Defaults() {
		t.Fatalf("got %+v, want defaults", s)
	}
	d := Defaults()
	if d.Theme != ThemeSystem || d.AudioQuality != Quality256 || d.Volume <= 0 || d.EngineIdleMinutes <= 0 ||
		d.ReduceEffects || d.Diagnostics != (Diagnostics{}) {
		t.Fatalf("unexpected defaults %+v", d)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	in := Settings{
		Theme:             ThemeDark,
		ReduceEffects:     true,
		Volume:            0.35,
		AudioQuality:      Quality64,
		CDMPath:           "/opt/google/chrome/WidevineCdm/_platform_specific/linux_x64/libwidevinecdm.so",
		EngineIdleMinutes: 3,
		Diagnostics:       Diagnostics{DisableDMABufRenderer: true, DisableCompositingMode: true},
	}
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
	if s != want {
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
	if s != Defaults() {
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
	if !got.ReduceEffects || got.Volume != 0 || st.Get() != got {
		t.Fatalf("update: got %+v, store %+v", got, st.Get())
	}
	reread, err := LoadSettings(path)
	if err != nil || reread != got {
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
	if st.Get() != Defaults() {
		t.Fatalf("in-memory settings changed after failed save: %+v", st.Get())
	}
}

func TestDiagnosticsApplyEnv(t *testing.T) {
	const dmabuf, compositing = "WEBKIT_DISABLE_DMABUF_RENDERER", "WEBKIT_DISABLE_COMPOSITING_MODE"
	t.Setenv(dmabuf, "")
	t.Setenv(compositing, "0") // explicit user value wins
	os.Unsetenv(dmabuf)

	if err := (Diagnostics{DisableDMABufRenderer: true, DisableCompositingMode: true}).ApplyEnv(); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv(dmabuf); v != "1" {
		t.Errorf("%s = %q, want 1", dmabuf, v)
	}
	if v := os.Getenv(compositing); v != "0" {
		t.Errorf("%s = %q, want user's 0 kept", compositing, v)
	}

	os.Unsetenv(dmabuf)
	if err := (Diagnostics{}).ApplyEnv(); err != nil {
		t.Fatal(err)
	}
	if _, ok := os.LookupEnv(dmabuf); ok {
		t.Errorf("%s set although the toggle is off", dmabuf)
	}
}
