package main

import (
	"strings"
	"testing"
)

const samplePO = `msgid ""
msgstr ""
"Language: fr\n"

#. placeholder {0}: n
#: src/a.tsx
msgid "{0, plural, one {# song} other {# songs}}"
msgstr "{0, plural, one {# morceau} other {# morceaux}}"

#: src/b.tsx
msgctxt "keyboard key"
msgid "Home"
msgstr ""

# translator note
#, fuzzy
msgid "Say \"hi\"\n"
"again"
msgstr "Dis"

#~ msgid "Gone"
#~ msgstr "Parti"
`

func TestParsePORoundTrip(t *testing.T) {
	f, err := parsePO([]byte(samplePO))
	if err != nil {
		t.Fatal(err)
	}
	if headerField(f.Header, "language") != "fr" {
		t.Fatalf("header %q", f.Header)
	}
	if len(f.Entries) != 4 {
		t.Fatalf("%d entries", len(f.Entries))
	}
	home := f.Entries[1]
	if !home.HasContext || home.Context != "keyboard key" || home.ID != "Home" {
		t.Fatalf("context entry %+v", home)
	}
	say := f.Entries[2]
	if say.ID != "Say \"hi\"\nagain" || !say.fuzzy() || say.Translator[0] != "translator note" {
		t.Fatalf("multi-line entry %+v", say)
	}
	if !f.Entries[3].Obsolete {
		t.Fatal("#~ entry not obsolete")
	}
	again, err := parsePO(formatPO(f))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Entries) != 3 || again.Entries[2].ID != say.ID || again.Entries[0].Str != f.Entries[0].Str {
		t.Fatalf("round trip lost entries: %+v", again.Entries)
	}
	if strings.Contains(string(formatPO(f)), "Gone") {
		t.Fatal("obsolete entry written")
	}
}

func TestImportLocaleTagsAppleKeysAndReportsMissing(t *testing.T) {
	f, _ := parsePO([]byte(samplePO))
	sona := &sonaCatalogue{parsed: map[msgKey]message{}}
	for _, e := range f.Entries[:2] {
		sona.entries = append(sona.entries, e)
		sona.parsed[e.key()] = mustSona(t, e.ID)
	}
	ms := []mapping{
		{Message: "{0, plural, one {# song} other {# songs}}", Key: "S", Plural: true, Placeholders: map[string]string{"count": "0"}},
		{Message: "Home", Context: "keyboard key", Key: "Gone.Key"},
	}
	b := appleBundle{"S.one": "@@count@@ morceau", "S.other": "@@count@@ morceaux"}
	entries, missing, invalid := importLocale(b, ms, sona)
	if len(entries) != 1 || len(missing) != 1 || missing[0] != "Gone.Key" || len(invalid) != 0 {
		t.Fatalf("entries %d missing %v invalid %v", len(entries), missing, invalid)
	}
	e := entries[0]
	if e.Extracted[0] != "apple-key: S" || e.Str != "{0, plural, one {# morceau} other {# morceaux}}" {
		t.Fatalf("entry %+v", e)
	}
	if got := missingKeys(ms, b); len(got) != 1 || got[0] != "Gone.Key" {
		t.Fatalf("missingKeys = %v", got)
	}
}

func TestComputeCompletenessLayersAppleOverCommunity(t *testing.T) {
	k := func(ids ...string) map[msgKey]bool {
		m := map[msgKey]bool{}
		for _, id := range ids {
			m[msgKey{ID: id}] = true
		}
		return m
	}
	apple := map[string]map[msgKey]bool{"de-DE": k("a", "b"), "fr-FR": k("a")}
	community := map[string]map[msgKey]bool{"de": k("b", "c", "d", "e", "f", "g", "h", "i", "j"), "eo": k("a")}
	c := computeCompleteness(10, apple, community)
	de := c.Locales["de-DE"]
	if de.Apple != 2 || de.Community != 8 || de.Share != 1 || !de.Selectable {
		t.Fatalf("de-DE %+v", de) // b counted once, from Apple
	}
	if fr := c.Locales["fr-FR"]; fr.Share != 0.1 || fr.Selectable {
		t.Fatalf("fr-FR %+v", fr)
	}
	if eo := c.Locales["eo"]; eo.Community != 1 || eo.Apple != 0 {
		t.Fatalf("eo %+v", eo)
	}
	// The community's own tag is a locale too, without Apple's layer.
	if de := c.Locales["de"]; de.Apple != 0 || de.Community != 9 {
		t.Fatalf("de %+v", de)
	}
	// Nine of ten is the threshold.
	c = computeCompleteness(10, nil, map[string]map[msgKey]bool{"it": k("a", "b", "c", "d", "e", "f", "g", "h", "i")})
	if !c.Locales["it"].Selectable {
		t.Fatalf("90%% not selectable: %+v", c.Locales["it"])
	}
}
