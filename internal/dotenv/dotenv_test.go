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
