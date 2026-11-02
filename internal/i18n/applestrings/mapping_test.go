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
	b, err := os.ReadFile("../../../frontend/src/shared/i18n/apple-keys.json")
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
	f, err := po.Read("../../../frontend/src/shared/i18n/locales/en.po")
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

func TestDecodeMappingsRefusesUnknownFields(t *testing.T) {
	if _, err := DecodeMappings([]byte(`[{"message":"Add","key":"A","text":"Ajouter"}]`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}
