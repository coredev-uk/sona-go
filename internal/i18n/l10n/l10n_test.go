package l10n

import (
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"
)

const testSource = `msgid ""
msgstr ""
"Language: en\n"

msgid "Play"
msgstr ""

#, go-format
msgid "%d songs"
msgstr ""

#, go-format
msgid "%s by %s"
msgstr ""

#, go-format
msgid "%s of %d"
msgstr ""

msgid "Unsure"
msgstr ""

msgid "Untranslated"
msgstr ""
`

const testFrench = `msgid ""
msgstr ""
"Language: fr\n"

msgid "Play"
msgstr "Lire"

#, go-format
msgid "%d songs"
msgstr "%d morceaux"

#, go-format
msgid "%s by %s"
msgstr "%[2]s : %[1]s"

#, go-format
msgid "%s of %d"
msgstr "%s sur %s"

#, fuzzy
msgid "Unsure"
msgstr "Incertain"

msgid "Gone from the source"
msgstr "Disparu"
`

// useCatalogues swaps the built-in catalogues for files (name → text,
// under locales/) for the test, with an empty cache.
func useCatalogues(t *testing.T, files map[string]string) {
	t.Helper()
	fsys := fstest.MapFS{}
	for name, text := range files {
		fsys["locales/"+name] = &fstest.MapFile{Data: []byte(text)}
	}
	reset := func(f fs.FS) {
		catMu.Lock()
		defer catMu.Unlock()
		catalogues, cats, tags, source = f, nil, nil, nil
	}
	old := catalogues
	reset(fsys)
	t.Cleanup(func() { reset(old) })
}

func TestFor(t *testing.T) {
	useCatalogues(t, map[string]string{"en.po": testSource, "fr.po": testFrench, "pt-BR.po": testSource})
	if got := Locales(); !slices.Equal(got, []string{"fr", "pt-BR"}) {
		t.Fatalf("Locales = %q", got)
	}
	p := For([]string{"fr-CA"})
	if p.Tag() != "fr" {
		t.Fatalf("fr-CA: catalogue %q", p.Tag())
	}
	for m, want := range map[Message]string{
		"Play":         "Lire",
		"Untranslated": "Untranslated",
		// Fuzzy: not reviewed, so not shown.
		"Unsure": "Unsure",
		// Its verbs (%s %s) differ from the English (%d %s): dropped.
		"%s of %d": "%s of %d",
	} {
		if got := p.T(m); got != want {
			t.Errorf("T(%q) = %q, want %q", m, got, want)
		}
	}
	if got := p.Sprintf("%d songs", 3); got != "3 morceaux" {
		t.Errorf("Sprintf = %q", got)
	}
	// A translation may reorder the verbs.
	if got := p.Sprintf("%s by %s", "Song", "Artist"); got != "Artist : Song" {
		t.Errorf("reordered Sprintf = %q", got)
	}
	if For([]string{"fr"}) != p {
		t.Error("a catalogue is loaded once")
	}
	for _, pref := range [][]string{nil, {"de"}, {"en-GB", "fr"}, {""}} {
		if got := For(pref); got != English {
			t.Errorf("For(%q) = %q, want English", pref, got.Tag())
		}
	}
	if got := For([]string{"de", "pt"}).Tag(); got != "pt-BR" {
		t.Errorf("de, pt: %q", got)
	}
}

func TestNilPrinterIsEnglish(t *testing.T) {
	var p *Printer
	if p.Tag() != SourceLocale || p.T("Play") != "Play" || p.Sprintf("%d songs", 2) != "2 songs" {
		t.Error("nil Printer isn't English")
	}
}

func TestMatchLocale(t *testing.T) {
	available := []string{"de", "fr-CA", "fr-FR", "pt-BR", "zh-Hant"}
	for _, tc := range []struct {
		preferred []string
		want      string
	}{
		{[]string{"fr-FR"}, "fr-FR"},
		{[]string{"FR-ca"}, "fr-CA"},
		{[]string{"de-AT"}, "de"},
		{[]string{"fr"}, "fr-CA"},
		{[]string{"pt-PT"}, "pt-BR"},
		{[]string{"zh-Hant-TW"}, "zh-Hant"},
		{[]string{"ja", "de"}, "de"},
		{[]string{"en-US", "de"}, ""},
		{[]string{" ", "de"}, "de"},
		{nil, ""},
	} {
		if got := MatchLocale(tc.preferred, available); got != tc.want {
			t.Errorf("MatchLocale(%q) = %q, want %q", tc.preferred, got, tc.want)
		}
	}
}

func TestVerbs(t *testing.T) {
	for s, want := range map[string][]string{
		"%d songs":        {"d"},
		"%[2]s : %[1]s":   {"s", "s"},
		"%-5.2f%% of %q":  {"f", "q"},
		"100%%":           nil,
		"no verbs at all": nil,
	} {
		if got := Verbs(s); !slices.Equal(got, want) {
			t.Errorf("Verbs(%q) = %q, want %q", s, got, want)
		}
	}
}

// Locales lists the tags of the translated catalogues built in (English,
// the source, is not one of them).
func Locales() []string {
	catMu.Lock()
	defer catMu.Unlock()
	loadIndexLocked()
	return slices.Clone(tags)
}

// Match is the UI's locale matching as well: the cases of v0.2.0's
// shared/i18n/locale.test.ts.
func TestMatch(t *testing.T) {
	available := []string{"en", "pt-PT", "pt-BR", "zh-Hant"}
	for _, tc := range []struct {
		preferred []string
		want      string
		ok        bool
	}{
		{[]string{"pt-br"}, "pt-BR", true},
		{[]string{"en-GB"}, "en", true},
		{[]string{"pt-AO"}, "pt-PT", true},
		{[]string{"zh-Hant-TW"}, "zh-Hant", true},
		{[]string{"", "de-DE", "pt-BR", "en"}, "pt-BR", true},
		{[]string{"de-DE", "fr"}, "", false},
		{nil, "", false},
	} {
		got, ok := Match(tc.preferred, available)
		if got != tc.want || ok != tc.ok {
			t.Errorf("Match(%q) = %q, %v; want %q, %v", tc.preferred, got, ok, tc.want, tc.ok)
		}
	}
}
