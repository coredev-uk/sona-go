package config

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// custom returns s with a value away from the default in every setting.
func custom() Settings {
	s := Defaults()
	s.Theme = ThemeDark
	s.ThemeID = "graphite"
	s.ReduceEffects = true
	s.Volume = 0.3
	s.AudioQuality = Quality64
	s.EngineIdleMinutes = 30
	s.AutoPlay = false
	s.Scrobbling.LastFM = ScrobbleAccount{Enabled: true, Username: "listener"}
	s.Desktop = Desktop{CloseAction: CloseQuits, Autostart: true, StartPage: "/charts", ResumeLastRoute: true, LastRoute: "/album/1"}
	s.Window = Window{Width: 1600, Height: 1000, Maximised: true}
	s.Shortcuts = Shortcuts{"player.toggle": "Ctrl+P"}
	s.Updates.Check = false
	s.Integrations = Integrations{DiscordPresence: true}
	s.Audio.SoundCheck = true
	s.Audio.OutputDevice = "alsa_output.usb-dac"
	s.Audio.Equalizer = Equalizer{Enabled: true, Preset: "custom", Custom: []float64{3, 2, 1, 0, 0, 0, 0, 1, 2, 3}, Preamp: -2}
	s.MotionArtwork.Enabled = false
	s.Queue.ReplaceConfirm = ReplaceConfirm
	s.Storage.ArtCacheMiB = 2048
	s.Onboarding.Completed = true
	s.Concerts.City = "gb-manchester"
	s.Sidebar = Sidebar{OpenFolders: []string{"f.1"}, FoldedSections: []string{"apple"}}
	s.LyricsView = LyricsView{Translation: true, Pronunciation: true}
	return s.Normalized()
}

// local copies the computer- and session-specific settings from src.
func local(dst, src Settings) Settings {
	dst.Volume = src.Volume
	dst.Scrobbling, dst.Window = src.Scrobbling, src.Window
	dst.Desktop.Autostart, dst.Desktop.LastRoute = src.Desktop.Autostart, src.Desktop.LastRoute
	dst.Audio.OutputDevice = src.Audio.OutputDevice
	dst.Onboarding = src.Onboarding
	dst.Concerts = src.Concerts
	dst.Sidebar = src.Sidebar
	dst.LyricsView = src.LyricsView
	return dst
}

func TestExportImportRoundTrip(t *testing.T) {
	from := custom()
	b, err := Export(from)
	if err != nil {
		t.Fatal(err)
	}
	im, err := ParseExport(b)
	if err != nil {
		t.Fatal(err)
	}
	// Another computer: its own window, device and accounts.
	to := Defaults()
	to.Window = Window{Width: 1000, Height: 700}
	to.Audio.OutputDevice = "hdmi"
	to.Scrobbling.ListenBrainz = ScrobbleAccount{Enabled: true, Username: "other"}
	got := to
	im.Apply(&got)
	got = got.Normalized()
	if want := local(from, to); !reflect.DeepEqual(got, want) {
		t.Fatalf("imported\n%+v\nwant\n%+v", got, want)
	}
}

func TestExportLeavesOutLocalSettings(t *testing.T) {
	b, err := Export(custom())
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Format   string         `json:"format"`
		Version  int            `json:"version"`
		Settings map[string]any `json:"settings"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Format != ExportFormat || doc.Version != ExportVersion {
		t.Fatalf("header = %q v%d", doc.Format, doc.Version)
	}
	for _, k := range localKeys {
		parent, key, nested := strings.Cut(k, ".")
		m := doc.Settings
		if nested {
			m, _ = m[parent].(map[string]any)
		} else {
			key = parent
		}
		if _, ok := m[key]; ok {
			t.Errorf("export has %s", k)
		}
	}
	for _, leak := range []string{"manchester", "listener", "libwidevinecdm", "usb-dac", "/album/1"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("export contains %q", leak)
		}
	}
	if _, ok := doc.Settings["theme"]; !ok {
		t.Error("export lacks theme")
	}
}

// Settings must never hold secrets (they live in the keyring), so an export
// cannot carry one: no settings field may be named like a credential.
func TestSettingsHoldNoSecrets(t *testing.T) {
	secret := regexp.MustCompile(`(?i)token|secret|passw|cookie|session|apikey|api_key|credential|mut$`)
	var walk func(reflect.Type, string)
	walk = func(ty reflect.Type, path string) {
		for f := range ty.Fields() {
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" {
				name = f.Name
			}
			if secret.MatchString(name) {
				t.Errorf("settings field %s%s looks like a secret", path, name)
			}
			if f.Type.Kind() == reflect.Struct {
				walk(f.Type, path+name+".")
			}
		}
	}
	walk(reflect.TypeFor[Settings](), "")
}

func TestImportNormalisesAndIgnoresUnknownAndLocalKeys(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := st.Get()
	im, err := ParseExport([]byte(`{
		"format": "sona-settings", "version": 1, "futureTopLevel": 1,
		"settings": {
			"theme": "neon", "engineIdleMinutes": 99999999, "futureKey": {"a": 1},
			"volume": 0.01, "cdmPath": "/tmp/evil.so", "window": {"width": 9000},
			"desktop": {"closeAction": "quit", "startPage": "https://example.com", "autostart": true, "lastRoute": "/x"},
			"audio": {"outputDevice": "evil", "equalizer": {"enabled": true, "preset": "custom", "custom": [99, -99, 1], "preamp": 5}},
			"storage": {"artCacheMiB": 1},
			"queue": {"replaceConfirm": "sometimes"}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var prev Settings
	got, err := st.Update(func(s *Settings) { prev = *s; im.Apply(s) })
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults()
	want.EngineIdleMinutes = maxEngineIdleMinutes
	want.Desktop.CloseAction = CloseQuits
	want.Audio.Equalizer = Equalizer{Enabled: true, Preset: "custom", Custom: []float64{12, -12, 1, 0, 0, 0, 0, 0, 0, 0}, Preamp: 0}
	want.Storage.ArtCacheMiB = MinArtCacheMiB
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("imported\n%+v\nwant\n%+v", got, want)
	}
	if !reflect.DeepEqual(prev, before) {
		t.Fatal("Apply wrote into the previous settings")
	}
	wantChanged := []string{"audio.equalizer.custom", "audio.equalizer.enabled", "audio.equalizer.preset", "desktop.closeAction", "engineIdleMinutes", "storage.artCacheMiB"}
	if c := Changes(prev, got); !slices.Equal(c, wantChanged) {
		t.Fatalf("Changes = %v, want %v", c, wantChanged)
	}
}

// encoding/json matches keys case-insensitively, so a document can name a
// local setting in any spelling ("Volume" lands in Settings.Volume); none
// of them may be imported. The document carries every key of a fully
// customised Settings, local ones included, in three spellings.
func TestImportIgnoresLocalKeysInAnySpelling(t *testing.T) {
	from := custom()
	m, err := settingsMap(from)
	if err != nil {
		t.Fatal(err)
	}
	var respell func(map[string]any, func(string) string) map[string]any
	respell = func(m map[string]any, f func(string) string) map[string]any {
		out := map[string]any{}
		for k, v := range m {
			if sub, ok := v.(map[string]any); ok && k != "shortcuts" {
				v = respell(sub, f)
			}
			out[f(k)] = v
		}
		return out
	}
	for name, f := range map[string]func(string) string{
		"as exported": func(k string) string { return k },
		"lower":       strings.ToLower,
		"upper":       strings.ToUpper,
	} {
		body, err := json.Marshal(map[string]any{"format": ExportFormat, "version": ExportVersion, "settings": respell(m, f)})
		if err != nil {
			t.Fatal(err)
		}
		im, err := ParseExport(body)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		to := Defaults()
		to.Volume = 0.9
		got := to
		im.Apply(&got)
		if want := local(from, to); !reflect.DeepEqual(got.Normalized(), want) {
			t.Errorf("%s: imported\n%+v\nwant\n%+v", name, got, want)
		}
	}
	for _, doc := range []string{`{"Volume": 0.01}`, `{"VOLUME": 0.01}`} {
		im, err := ParseExport([]byte(`{"format":"sona-settings","version":1,"settings":` + doc + `}`))
		if err != nil {
			t.Fatal(err)
		}
		got := Defaults()
		if im.Apply(&got); got.Volume != Defaults().Volume {
			t.Errorf("%s: Volume = %v", doc, got.Volume)
		}
	}
}

func TestImportShortcutsReplaceOnlyWhenListed(t *testing.T) {
	cur := Defaults()
	cur.Shortcuts = Shortcuts{"player.toggle": "Ctrl+P"}
	for _, tc := range []struct {
		body string
		want Shortcuts
	}{
		{`{"shortcuts": {"search.focus": "Ctrl+L"}}`, Shortcuts{"search.focus": "Ctrl+L"}},
		{`{"shortcuts": {}}`, Shortcuts{}},
		{`{"theme": "dark"}`, Shortcuts{"player.toggle": "Ctrl+P"}},
	} {
		im, err := ParseExport([]byte(`{"format":"sona-settings","version":1,"settings":` + tc.body + `}`))
		if err != nil {
			t.Fatal(err)
		}
		got := cur.clone()
		im.Apply(&got)
		if got = got.Normalized(); !reflect.DeepEqual(got.Shortcuts, tc.want) {
			t.Errorf("%s: shortcuts = %v, want %v", tc.body, got.Shortcuts, tc.want)
		}
		if !reflect.DeepEqual(cur.Shortcuts, Shortcuts{"player.toggle": "Ctrl+P"}) {
			t.Fatalf("%s: Apply changed the source map", tc.body)
		}
	}
}

func TestParseExportRejects(t *testing.T) {
	for _, tc := range []struct {
		doc  string
		want error
		msg  string
	}{
		{doc: `not json`, want: ErrNotExport},
		{doc: `{"theme": "dark"}`, want: ErrNotExport}, // a settings.json, not an export
		{doc: `{"format": "sona-settings", "version": 0, "settings": {}}`, want: ErrNotExport},
		{doc: `{"format": "sona-settings", "version": 1}`, want: ErrNotExport},
		{doc: `{"format": "sona-settings", "version": 1, "settings": []}`, want: ErrNotExport},
		{doc: `{"format": "sona-settings", "version": 2, "settings": {}}`, want: ErrNewerExport},
		{doc: `{"format": "sona-settings", "version": 1, "settings": {"desktop": {"closeAction": 3}}}`, msg: "desktop.closeAction"},
		{doc: `{"format": "sona-settings", "version": 1, "settings": {"desktop": "quit"}}`, msg: "desktop"},
	} {
		_, err := ParseExport([]byte(tc.doc))
		switch {
		case err == nil:
			t.Errorf("%s: no error", tc.doc)
		case tc.want != nil && !errors.Is(err, tc.want):
			t.Errorf("%s: err = %v, want %v", tc.doc, err, tc.want)
		case tc.msg != "" && !strings.Contains(err.Error(), tc.msg):
			t.Errorf("%s: err = %v, want it to name %s", tc.doc, err, tc.msg)
		}
	}
}
