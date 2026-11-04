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

func TestExtractUIAddsMissingAndKeepsTheRest(t *testing.T) {
	const header = "msgid \"\"\nmsgstr \"\"\n\"Language: en\\n\"\n"
	existing := header + "\n#: src/x.tsx\nmsgid \"Albums\"\nmsgstr \"Albums\"\n\n#: src/old.tsx\nmsgid \"Only in Lingui\"\nmsgstr \"Only in Lingui\"\n"
	pseudo := header + "\n#: src/x.tsx\nmsgid \"Albums\"\nmsgstr \"\"\n"
	r := writeTree(t, map[string]string{
		"internal/ui/features/page/page.go": testSource,
		sourcePOPath:                        existing,
		pseudoPOPath:                        pseudo,
	})

	if err := runExtractUI(r, true); err == nil || !strings.Contains(err.Error(), `"Play"`) {
		t.Fatalf("check passed or says nothing useful: %v", err)
	}
	if b, _ := os.ReadFile(r.path(sourcePOPath)); string(b) != existing {
		t.Fatal("check wrote the catalogue")
	}

	if err := runExtractUI(r, false); err != nil {
		t.Fatal(err)
	}
	en, err := os.ReadFile(r.path(sourcePOPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(en), existing) {
		t.Errorf("what was there changed:\n%s", en)
	}
	f, err := po.Parse(en)
	if err != nil {
		t.Fatal(err)
	}
	have := map[po.Key]*po.Entry{}
	for _, e := range f.Entries {
		have[e.Key()] = e
	}
	for _, k := range []po.Key{{ID: "Only in Lingui"}, {ID: "Albums"}, {ID: "Play", Context: "verb"}, {ID: "Add music to collect it here."}} {
		if have[k] == nil {
			t.Errorf("en.po lacks %v", k)
		}
	}
	if e := have[po.Key{ID: "Play", Context: "verb"}]; e != nil && (e.Str != "Play" || !e.HasContext) {
		t.Errorf("Play entry %+v", e)
	}
	xa, _ := os.ReadFile(r.path(pseudoPOPath))
	if !strings.HasPrefix(string(xa), pseudo) || !strings.Contains(string(xa), "msgid \"Albums\"") {
		t.Errorf("en-XA.po:\n%s", xa)
	}
	if fx, err := po.Parse(xa); err != nil {
		t.Fatal(err)
	} else {
		for _, e := range fx.Entries {
			if e.ID == "Play" && e.Str != "" {
				t.Errorf("pseudo translation %q", e.Str)
			}
		}
	}

	// Now nothing is missing: check passes and a second run writes nothing.
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

// The repository's own calls are all in the catalogue: what CI's
// `extract -check` asserts, here for `go test`.
func TestRepositoryCatalogueHoldsEveryCall(t *testing.T) {
	if err := runExtractUI(repo("../.."), true); err != nil {
		t.Fatal(err)
	}
}

// Appending must not disturb a single byte of the catalogue Lingui wrote.
func TestAppendedKeepsBytes(t *testing.T) {
	raw, err := os.ReadFile("../../internal/ui/i18n/locales/en.po")
	if err != nil {
		t.Fatal(err)
	}
	c := &uiCatalogue{raw: raw, str: func(m *uiMessage) string { return m.ID }}
	out := c.appended([]*uiMessage{{Key: po.Key{ID: "Brand new"}, refs: []string{"internal/ui/x.go"}}})
	if !strings.HasPrefix(string(out), string(raw)) {
		t.Fatal("prefix changed")
	}
	if !strings.HasSuffix(string(out), "\n#: internal/ui/x.go\nmsgid \"Brand new\"\nmsgstr \"Brand new\"\n") {
		t.Errorf("appended entry:\n%s", out[len(raw):])
	}
}
