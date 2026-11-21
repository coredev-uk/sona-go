package config

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestForDiagnosticsWithholdsWhereThePersonIs(t *testing.T) {
	s := custom()
	s.Addons = Addons{"demo": {Enabled: true, Granted: []string{"playback.read"}, Values: map[string]string{"user": "someone"}}}
	m := ForDiagnostics(s)
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, leaked := range []string{"gb-manchester", "f.1", "alsa_output.usb-dac", "someone"} {
		if strings.Contains(text, leaked) {
			t.Errorf("the diagnostics settings still carry %q: %s", leaked, text)
		}
	}
	// What a bug report needs stays.
	for _, kept := range []string{`"theme":"dark"`, `"graphite"`, `"playback.read"`, `"foldedSections":["apple"]`, `"preamp":-2`} {
		if !strings.Contains(text, kept) {
			t.Errorf("the diagnostics settings lost %s: %s", kept, text)
		}
	}
	got, _ := m[DiagnosticsWithheldKey].([]string)
	for _, want := range []string{"concerts.city", "sidebar.openFolders", "audio.outputDevice", "addons.*.values"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s does not list %q: %v", DiagnosticsWithheldKey, want, got)
		}
	}
}

func TestForDiagnosticsLeavesTheSettingsAlone(t *testing.T) {
	s := custom()
	s.Addons = Addons{"demo": {Enabled: true, Values: map[string]string{"user": "someone"}}}
	_ = ForDiagnostics(s)
	if s.Concerts.City != "gb-manchester" || s.Addons["demo"].Values["user"] != "someone" || s.Audio.OutputDevice == "" {
		t.Fatalf("ForDiagnostics changed its argument: %+v", s)
	}
}
