package po

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

func TestParseRoundTrip(t *testing.T) {
	f, err := Parse([]byte(samplePO))
	if err != nil {
		t.Fatal(err)
	}
	if HeaderField(f.Header, "language") != "fr" {
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
	if say.ID != "Say \"hi\"\nagain" || !say.Fuzzy() || say.Translator[0] != "translator note" {
		t.Fatalf("multi-line entry %+v", say)
	}
	if !f.Entries[3].Obsolete {
		t.Fatal("#~ entry not obsolete")
	}
	again, err := Parse(Format(f))
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Entries) != 3 || again.Entries[2].ID != say.ID || again.Entries[0].Str != f.Entries[0].Str {
		t.Fatalf("round trip lost entries: %+v", again.Entries)
	}
	if strings.Contains(string(Format(f)), "Gone") {
		t.Fatal("obsolete entry written")
	}
}
