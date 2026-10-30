package services

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
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
		s.CDMPath = "/opt/chrome/libwidevinecdm.so"
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
	if cur.Theme != config.ThemeDark || cur.Window != config.Defaults().Window || cur.CDMPath != "" {
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
