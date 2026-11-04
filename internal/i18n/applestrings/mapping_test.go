package applestrings

import (
	"errors"
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

func TestMessageIDMatchesLingui(t *testing.T) {
	// Values from @lingui/message-utils generateMessageId.
	for _, c := range []struct{ msg, ctx, id string }{
		{"Add", "", "m16xKo"},
		{"Home", "keyboard key", "JudSqQ"},
		{"{count, plural, one {# song} other {# songs}}", "", "rr12-a"},
		{"Café", "", "cZM_QB"},
	} {
		if got := MessageID(c.msg, c.ctx); got != c.id {
			t.Errorf("MessageID(%q, %q) = %q, want %q", c.msg, c.ctx, got, c.id)
		}
	}
}

func TestConvertKeepsOnlyMappedCheckedMessages(t *testing.T) {
	ms := []Mapping{
		{Message: "Add", Key: "A"},
		{Message: "Play {name}", Key: "P", Placeholders: map[string]string{"title": "name"}},
		{Message: "Shuffle {name}", Key: "S", Placeholders: map[string]string{"title": "name"}},
		{Message: "{n, plural, one {# song} other {# songs}}", Key: "N", Plural: true, Placeholders: map[string]string{"count": "n"}},
		{Message: "Long", Key: "L"},
		{Message: "Gone", Key: "G"},
	}
	b := Bundle{
		"A":       "Ajouter",
		"P":       "Lire",                    // drops {name}
		"S":       "Mélanger @@other@@",      // unknown placeholder
		"N.one":   "@@count@@ morceau",       //
		"N.other": "@@count@@ morceaux",      //
		"L":       strings.Repeat("x", 5000), // over the cap
		"X":       "Not mapped",              // never kept
	}
	c := Convert(b, ms)
	want := map[string]string{
		MessageID("Add", ""): "Ajouter",
		MessageID("{n, plural, one {# song} other {# songs}}", ""): "{n, plural, one {# morceau} other {# morceaux}}",
	}
	if !maps.Equal(c.Messages, want) {
		t.Fatalf("messages = %v, want %v", c.Messages, want)
	}
	if len(c.Missing) != 1 || c.Missing[0] != "G" {
		t.Fatalf("missing = %v", c.Missing)
	}
	if len(c.Invalid) != 3 {
		t.Fatalf("invalid = %v", c.Invalid)
	}
	if _, err := ms[5].Translate(b); !errors.Is(err, ErrMissing) {
		t.Fatalf("Translate missing key: %v", err)
	}
}

// The committed mapping must fit the committed English catalogue: every
// mapped message exists, parses, and renames onto its own arguments.
func TestEmbeddedMappingFitsCatalogue(t *testing.T) {
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile("../../ui/i18n/locales/apple-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := DecodeMappings(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Mappings) != len(ms) {
		t.Fatalf("%d of %d mapped messages are in en.po", len(cat.Mappings), len(ms))
	}
	f, err := po.Read("../../ui/i18n/locales/en.po")
	if err != nil {
		t.Fatal(err)
	}
	sona := map[po.Key]Message{}
	n := 0
	for _, e := range f.Entries {
		if e.Obsolete {
			continue
		}
		n++
		if m, ok := ParseSona(e.ID); ok {
			sona[e.Key()] = m
		}
	}
	if err := ValidateMappings(ms, sona); err != nil {
		t.Fatal(err)
	}
	if cat.Messages != n || n == 0 || cat.Version == "" {
		t.Fatalf("catalogue %d messages (want %d), version %q", cat.Messages, n, cat.Version)
	}
}

// Moving the UI from Lingui to Go keeps every mapping v0.2.0 shipped: its
// 147 messages are all still mapped to the same keys and placeholders, so
// Apple's translations reach every message they did before. A message
// dropped from the UI drops its mapping with it (the test above), so the
// list here only shrinks when a v0.2.0 feature is retired on purpose.
func TestV020MappingsStillHold(t *testing.T) {
	const v020 = 147
	old, err := os.ReadFile("testdata/v0.2.0-apple-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	was, err := DecodeMappings(old)
	if err != nil {
		t.Fatal(err)
	}
	if len(was) != v020 {
		t.Fatalf("testdata holds %d mappings, v0.2.0 had %d", len(was), v020)
	}
	b, err := os.ReadFile("../../ui/i18n/locales/apple-keys.json")
	if err != nil {
		t.Fatal(err)
	}
	now, err := DecodeMappings(b)
	if err != nil {
		t.Fatal(err)
	}
	have := map[po.Key]Mapping{}
	for _, m := range now {
		have[m.MsgKey()] = m
	}
	for _, m := range was {
		got, ok := have[m.MsgKey()]
		if !ok || got.Key != m.Key || got.Plural != m.Plural || !maps.Equal(got.Placeholders, m.Placeholders) {
			t.Errorf("v0.2.0 mapped %q to %s (%v); now %+v", m.Message, m.Key, m.Placeholders, got)
		}
	}
	cat, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Mappings) < v020 {
		t.Errorf("%d mapped messages reach the app, v0.2.0 had %d", len(cat.Mappings), v020)
	}
}

func TestDecodeMappingsRefusesUnknownFields(t *testing.T) {
	if _, err := DecodeMappings([]byte(`[{"message":"Add","key":"A","text":"Ajouter"}]`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}
