package extract

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// The rules on a small tree (testdata/ok): typed constants and variables
// (grouped or not, l10n imported under another name or the package
// itself), conversions, maps and slices of messages, raw strings and
// joined literals, comments as notes, every OS's files; never tests,
// generated code, testdata or dependencies.
func TestExtractRules(t *testing.T) {
	f, err := Extract("testdata/ok")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/ok.po")
	if err != nil {
		t.Fatal(err)
	}
	if got := po.Format(f); !bytes.Equal(got, want) {
		t.Errorf("catalogue:\n%s\nwant:\n%s", got, want)
	}
	// Formatting is stable: the parsed catalogue writes the same bytes.
	parsed, err := po.Parse(want)
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(parsed, f); len(d) > 0 {
		t.Errorf("Diff against itself: %q", d)
	}
}

// A marked value the catalogue can't hold fails with its file and line.
func TestExtractRejectsNonLiterals(t *testing.T) {
	_, err := Extract("testdata/bad")
	if err == nil {
		t.Fatal("no error")
	}
	msg := err.Error()
	for _, want := range []string{
		"cmd/cmd.go:13: a message must be a string literal or a marked message's name, not a call",
		"cmd/cmd.go:15: a message must be a string literal or a marked message's name, not an expression",
		"cmd/cmd.go:17: untyped is not declared as an l10n.Message",
		"cmd/cmd.go:19: a message must be a string literal or a marked message's name, not a call",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error lacks %q:\n%s", want, msg)
		}
	}
	if n := strings.Count(msg, "\n") + 1; n != 4 {
		t.Errorf("%d errors, want 4:\n%s", n, msg)
	}
}

func TestDiff(t *testing.T) {
	old := &po.File{Entries: []*po.Entry{
		{ID: "Kept", References: []string{"a.go"}},
		{ID: "Moved", References: []string{"a.go"}},
		{ID: "Removed"},
	}}
	next := &po.File{Entries: []*po.Entry{
		{ID: "Added"},
		{ID: "Kept", References: []string{"a.go"}},
		{ID: "Moved", References: []string{"b.go"}},
	}}
	got := strings.Join(Diff(old, next), "\n")
	if want := "+ \"Added\"\n~ \"Moved\"\n- \"Removed\""; got != want {
		t.Errorf("Diff = %q, want %q", got, want)
	}
}

// The committed catalogue is what extraction writes from the source, so
// `go test ./...` fails when a message changed without it.
func TestCatalogueUpToDate(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Extract(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(SourcePath)))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, po.Format(f)) {
		return
	}
	diff := []string{"formatting"}
	if old, err := po.Parse(got); err == nil {
		if d := Diff(old, f); len(d) > 0 {
			diff = d
		}
	}
	t.Errorf("%s is out of date; run `%s`:\n  %s", SourcePath, Command, strings.Join(diff, "\n  "))
}
