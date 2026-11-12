package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	for _, body := range []string{"one", "two, longer"} {
		if err := WriteFileAtomic(path, []byte(body), 0o640); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != body {
			t.Fatalf("read = %q, %v; want %q", got, err, body)
		}
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o640 {
			t.Errorf("perm = %v, want 0640", fi.Mode().Perm())
		}
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("directory holds %d entries, want only the file", len(ents))
	}
}

func TestWriteFileAtomicFailureLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	// A directory at path makes the final rename fail.
	path := filepath.Join(dir, "d")
	if err := os.MkdirAll(filepath.Join(path, "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("x"), 0o600); err == nil {
		t.Fatal("expected an error replacing a directory")
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("directory holds %d entries, want only d", len(ents))
	}
}

func TestIsTemp(t *testing.T) {
	if !IsTemp(".a.json.123.tmp") || IsTemp("a.json") || IsTemp(".hidden") || IsTemp("x.tmp") {
		t.Error("IsTemp misclassifies")
	}
}
