package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

const testSource = `package page

import (
	"strings"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

func view(n int) {
	_ = i18n.T("Albums")
	_ = i18n.T("{0, plural, one {# album} other {# albums}}", "0", n)
	_ = i18n.TC("verb", "Play")
	_ = i18n.T("Add music " + "to collect it here.")
	_ = strings.ToLower("i18n.T(\"not a call of ours\")")
	label := i18n.T
	_ = label
}
`

func writeTree(t *testing.T, files map[string]string) repo {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return repo(root)
}

func keys(msgs []*uiMessage) []string {
	var out []string
	for _, m := range msgs {
		s := m.ID
		if m.HasContext {
			s = m.Context + "|" + s
		}
		out = append(out, s)
	}
	return out
}

func TestScanUICollectsLiterals(t *testing.T) {
	r := writeTree(t, map[string]string{
		"internal/ui/features/page/page.go":      testSource,
		"internal/ui/features/page/page_test.go": strings.ReplaceAll(testSource, "Albums", "From a test"),
		"internal/ui/features/page/testdata/x.go": `package x; import "github.com/coredev-uk/sona-go/internal/ui/i18n"
var _ = i18n.T("From testdata")`,
		"internal/ui/features/page/alias.go": `package page

import loc "github.com/coredev-uk/sona-go/internal/ui/i18n"

var _ = loc.T("Aliased")
`,
		"internal/ui/i18n/errors.go": `package i18n

func ErrorText() string { return T("Something went wrong.") + TC("ctx", "Bare") }
`,
	})
	msgs, err := scanUI(string(r))
	if err != nil {
		t.Fatal(err)
	}
	// Sorted by msgid, then context.
	want := []string{
		"Add music to collect it here.",
		"Albums",
		"Aliased",
		"ctx|Bare",
		"verb|Play",
		"Something went wrong.",
		"{0, plural, one {# album} other {# albums}}",
	}
	if got := keys(msgs); !slices.Equal(got, want) {
		t.Errorf("messages %q, want %q", got, want)
	}
	for _, m := range msgs {
		if m.ID == "Albums" && strings.Join(m.refs, ",") != "internal/ui/features/page/page.go" {
			t.Errorf("Albums refs %v", m.refs)
		}
	}
}

func TestScanUIRejectsNonLiterals(t *testing.T) {
	r := writeTree(t, map[string]string{
		"internal/ui/features/page/page.go": `package page

import "github.com/coredev-uk/sona-go/internal/ui/i18n"

const title = "Albums"

func view(s string) {
	_ = i18n.T(title)
	_ = i18n.T(s, "k", 1)
	_ = i18n.TC("ok", s)
	_ = i18n.T()
	_ = i18n.T("fine")
}
`,
	})
	msgs, err := scanUI(string(r))
	if err == nil {
		t.Fatal("non-literals accepted")
	}
	for _, want := range []string{"page.go:8", "page.go:9", "page.go:10", "page.go:11"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %s: %v", want, err)
		}
	}
	if len(msgs) != 1 || msgs[0].ID != "fine" {
		t.Errorf("messages %v", keys(msgs))
	}
}

func TestExtractUIWritesExactCatalogues(t *testing.T) {
	stale := "msgid \"\"\nmsgstr \"\"\n\"Language: en\\n\"\n\n#: src/x.tsx\nmsgid \"Albums\"\nmsgstr \"Albums\"\n\n#: src/old.tsx\nmsgid \"Only in Lingui\"\nmsgstr \"Only in Lingui\"\n"
	r := writeTree(t, map[string]string{
		"internal/ui/features/page/page.go": testSource,
		sourcePOPath:                        stale,
		pseudoPOPath:                        stale,
	})

	err := runExtractUI(r, true)
	if err == nil {
		t.Fatal("check passed on a stale catalogue")
	}
	for _, want := range []string{`+ "verb"/"Play"`, `- "Only in Lingui"`, `~ "Albums"`, sourcePOPath, pseudoPOPath} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("check says nothing of %s: %v", want, err)
		}
	}
	if b, _ := os.ReadFile(r.path(sourcePOPath)); string(b) != stale {
		t.Fatal("check wrote the catalogue")
	}

	if err := runExtractUI(r, false); err != nil {
		t.Fatal(err)
	}
	en, err := po.Read(r.path(sourcePOPath))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := entryKeys(en), []string{"Add music to collect it here.", "Albums", "Play", "{0, plural, one {# album} other {# albums}}"}; !slices.Equal(got, want) {
		t.Errorf("en.po holds %q, want %q", got, want)
	}
	have := map[po.Key]*po.Entry{}
	for _, e := range en.Entries {
		have[e.Key()] = e
	}
	if have[po.Key{ID: "Only in Lingui"}] != nil {
		t.Error("en.po keeps a message no call asks for")
	}
	if e := have[po.Key{ID: "Play", Context: "verb"}]; e == nil || e.Str != "Play" || !e.HasContext {
		t.Errorf("Play entry %+v", e)
	}
	if e := have[po.Key{ID: "Albums"}]; e == nil || strings.Join(e.References, ",") != "internal/ui/features/page/page.go" {
		t.Errorf("Albums entry %+v", e)
	}
	if got := po.HeaderField(en.Header, "Language"); got != "en" {
		t.Errorf("en.po language %q", got)
	}
	xa, err := po.Read(r.path(pseudoPOPath))
	if err != nil {
		t.Fatal(err)
	}
	if got := po.HeaderField(xa.Header, "Language"); got != "en-XA" || len(xa.Entries) != len(en.Entries) {
		t.Errorf("en-XA.po language %q, %d entries", got, len(xa.Entries))
	}
	for _, e := range xa.Entries {
		if e.Str != "" {
			t.Errorf("pseudo translation %q of %q", e.Str, e.ID)
		}
	}

	// Now both are exact: check passes and a second run writes the same bytes.
	if err := runExtractUI(r, true); err != nil {
		t.Fatalf("check after extract: %v", err)
	}
	before, _ := os.ReadFile(r.path(sourcePOPath))
	if err := runExtractUI(r, false); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(r.path(sourcePOPath)); string(after) != string(before) {
		t.Error("a second extract changed the catalogue")
	}
}

func entryKeys(f *po.File) []string {
	var out []string
	for _, e := range f.Entries {
		out = append(out, e.ID)
	}
	return out
}

// A mapping whose message no call asks for any more fails extraction: the
// app would carry a mapping that never applies.
func TestExtractUIFailsOnMappingOfADroppedMessage(t *testing.T) {
	r := writeTree(t, map[string]string{
		"internal/ui/features/page/page.go": testSource,
		mappingPath: `[{"message":"Albums","key":"A"},{"message":"Gone","key":"G"}]`,
	})
	if err := runExtractUI(r, false); err == nil || !strings.Contains(err.Error(), "Gone") {
		t.Fatalf("mapping of a dropped message accepted: %v", err)
	}
}

// The repository's catalogues are exactly what the calls ask for: what
// CI's `extract -check` asserts, here for `go test`.
func TestRepositoryCatalogueIsExact(t *testing.T) {
	if err := runExtractUI(repo("../.."), true); err != nil {
		t.Fatal(err)
	}
}
