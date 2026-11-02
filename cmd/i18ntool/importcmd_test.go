package main

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

func TestImportLocaleTagsAppleKeysAndReportsMissing(t *testing.T) {
	sona := &sonaCatalogue{parsed: map[po.Key]applestrings.Message{}}
	for _, e := range []*po.Entry{
		{ID: "{0, plural, one {# song} other {# songs}}", References: []string{"src/a.tsx"}},
		{ID: "Home", Context: "keyboard key", HasContext: true},
		{ID: "Play {name}"},
	} {
		sona.entries = append(sona.entries, e)
		sona.parsed[e.Key()] = mustSona(t, e.ID)
	}
	ms := []applestrings.Mapping{
		{Message: "{0, plural, one {# song} other {# songs}}", Key: "S", Plural: true, Placeholders: map[string]string{"count": "0"}},
		{Message: "Home", Context: "keyboard key", Key: "Gone.Key"},
		{Message: "Play {name}", Key: "P", Placeholders: map[string]string{"title": "name"}},
	}
	b := applestrings.Bundle{"S.one": "@@count@@ morceau", "S.other": "@@count@@ morceaux", "P": "Lecture"}
	entries, missing, invalid := importLocale(b, ms, sona)
	if len(entries) != 1 || len(missing) != 1 || missing[0] != "Gone.Key" || len(invalid) != 1 {
		t.Fatalf("entries %d missing %v invalid %v", len(entries), missing, invalid)
	}
	e := entries[0]
	if e.Extracted[0] != "apple-key: S" || e.Str != "{0, plural, one {# morceau} other {# morceaux}}" || e.References[0] != "src/a.tsx" {
		t.Fatalf("entry %+v", e)
	}
	if got := missingKeys(ms, b); len(got) != 1 || got[0] != "Gone.Key" {
		t.Fatalf("missingKeys = %v", got)
	}
}
