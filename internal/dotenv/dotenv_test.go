package dotenv

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadAndSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if v, err := Read(path, "A"); v != "" || err != nil {
		t.Fatalf("missing file: %q, %v", v, err)
	}
	if err := os.WriteFile(path, []byte("# c\nexport A=\"one\"\nB = 'two'\n\nC=x=y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"A": "one", "B": "two", "C": "x=y", "D": ""} {
		if v, err := Read(path, key); v != want || err != nil {
			t.Errorf("Read(%s) = %q, %v; want %q", key, v, err, want)
		}
	}
	if err := Set(path, "A", "new"); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "D", "d"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if want := "# c\nB = 'two'\nC=x=y\nA=new\nD=d\n"; string(b) != want {
		t.Errorf("file = %q, want %q", b, want)
	}
}

func TestSetWritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "main.env")
	link := filepath.Join(dir, ".env")
	if err := os.WriteFile(real, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks:", err)
	}
	if err := Set(link, "B", "2"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("the symlink was replaced: %v, %v", fi, err)
	}
	if b, _ := os.ReadFile(real); string(b) != "A=1\nB=2\n" {
		t.Errorf("target = %q", b)
	}
}
