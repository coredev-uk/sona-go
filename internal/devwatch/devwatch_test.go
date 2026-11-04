package devwatch

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestChanged(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "pkg", "a.go"), filepath.Join(root, "pkg", "b.go")
	write(t, a, "package pkg")
	w := New(Options{Roots: []string{root}, SkipDir: func(n string) bool { return n == "skip" }, Match: func(p string) bool { return strings.HasSuffix(p, ".go") }})

	if got := w.Changed(); got != nil {
		t.Fatalf("baseline reported %v", got)
	}
	if got := w.Changed(); got != nil {
		t.Fatalf("nothing changed, got %v", got)
	}

	write(t, b, "package pkg")
	write(t, filepath.Join(root, "pkg", "notes.txt"), "not matched")
	write(t, filepath.Join(root, "skip", "c.go"), "skipped")
	if got := w.Changed(); !slices.Equal(got, []string{b}) {
		t.Fatalf("created: got %v, want [%s]", got, b)
	}

	// Same size, same second: the modification time is what differs.
	write(t, a, "package xxx")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(a, later, later); err != nil {
		t.Fatal(err)
	}
	if got := w.Changed(); !slices.Equal(got, []string{a}) {
		t.Fatalf("written: got %v, want [%s]", got, a)
	}

	if err := os.Remove(b); err != nil {
		t.Fatal(err)
	}
	if got := w.Changed(); !slices.Equal(got, []string{b}) {
		t.Fatalf("removed: got %v, want [%s]", got, b)
	}
}

func TestFileRoot(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "tokens.json")
	write(t, f, "{}")
	w := New(Options{Roots: []string{f}})
	w.Changed()
	write(t, f, `{"a": 1}`)
	if got := w.Changed(); !slices.Equal(got, []string{f}) {
		t.Fatalf("got %v", got)
	}
}
