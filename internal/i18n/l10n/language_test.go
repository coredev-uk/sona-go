package l10n

import (
	"slices"
	"testing"
)

func TestEnvLanguages(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{"LANG", map[string]string{"LANG": "fr_FR.UTF-8"}, []string{"fr-FR"}},
		{"modifier", map[string]string{"LANG": "sr_RS@latin"}, []string{"sr-RS"}},
		{"LC_MESSAGES over LANG", map[string]string{"LANG": "de_DE.UTF-8", "LC_MESSAGES": "it_IT.UTF-8"}, []string{"it-IT"}},
		{"LC_ALL over both", map[string]string{"LANG": "de_DE", "LC_MESSAGES": "it_IT", "LC_ALL": "es_ES"}, []string{"es-ES"}},
		{
			"LANGUAGE first, then the locale",
			map[string]string{"LANG": "en_GB.UTF-8", "LANGUAGE": "de:fr_CA::de"},
			[]string{"de", "fr-CA", "en-GB"},
		},
		{"LANGUAGE naming the locale", map[string]string{"LANG": "fr_FR", "LANGUAGE": "fr_FR:de"}, []string{"fr-FR", "de"}},
		// The C and POSIX locales mean untranslated: LANGUAGE is ignored.
		{"C", map[string]string{"LANG": "C", "LANGUAGE": "de"}, nil},
		{"C.UTF-8", map[string]string{"LC_ALL": "C.UTF-8", "LANG": "fr_FR"}, nil},
		{"POSIX", map[string]string{"LANG": "POSIX"}, nil},
		{"nothing set", map[string]string{"LANGUAGE": "de"}, nil},
	} {
		got := envLanguages(func(k string) string { return tc.env[k] })
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestParseAppleLanguages(t *testing.T) {
	out := "(\n    \"en-GB\",\n    fr,\n    \"zh-Hans-CN\",\n    \"en-GB\"\n)\n"
	if got := parseAppleLanguages(out); !slices.Equal(got, []string{"en-GB", "fr", "zh-Hans-CN"}) {
		t.Errorf("parseAppleLanguages = %q", got)
	}
	if got := parseAppleLanguages("()\n"); got != nil {
		t.Errorf("empty list = %q", got)
	}
}

func TestLanguage(t *testing.T) {
	useCatalogues(t, map[string]string{"en.po": testSource, "fr.po": testFrench})
	for _, k := range []string{"LANGUAGE", "LC_ALL", "LC_MESSAGES"} {
		t.Setenv(k, "")
	}
	t.Setenv("LANG", "de_DE.UTF-8")

	l := NewLanguage("")
	if got := l.Preferred(); !slices.Equal(got, []string{"de-DE"}) {
		t.Fatalf("Automatic: Preferred = %q", got)
	}
	if l.Printer() != English {
		t.Fatalf("de-DE has no catalogue: %q", l.Printer().Tag())
	}
	calls := 0
	cancel := l.OnChange(func() { calls++ })

	l.SetSystem([]string{"fr-BE", "de-DE"})
	if calls != 1 || l.Printer().Tag() != "fr" {
		t.Fatalf("system languages: %d calls, %q", calls, l.Printer().Tag())
	}
	// The setting wins over the system's list, and alone.
	l.SetSetting("en")
	if calls != 2 || l.Printer() != English || !slices.Equal(l.Preferred(), []string{"en"}) {
		t.Fatalf("setting: %d calls, %q, %q", calls, l.Printer().Tag(), l.Preferred())
	}
	// No change, no call.
	l.SetSetting("en")
	l.SetSystem([]string{"ja"})
	if calls != 2 {
		t.Fatalf("unchanged preference called %d times", calls)
	}
	cancel()
	l.SetSetting("")
	if calls != 2 {
		t.Fatal("called after cancel")
	}
	if got := l.Preferred(); !slices.Equal(got, []string{"ja"}) {
		t.Fatalf("back to Automatic: %q", got)
	}

	var none *Language
	if none.Printer() != English {
		t.Error("nil Language isn't English")
	}
}
