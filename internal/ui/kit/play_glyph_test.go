package kit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPlayMarksAreAllSolid keeps the one rule in docs/design.md: a play
// triangle is drawn by PlayGlyph (solid), never by an unfilled theme.Glyph, so
// a disc, a wash and a number cell never mix outline and solid triangles.
func TestPlayMarksAreAllSolid(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		if filepath.Base(p) == "button.go" && filepath.Base(filepath.Dir(p)) == "kit" {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(b), `theme.Glyph(c, "play"`) {
			t.Errorf("%s draws the play triangle with theme.Glyph; use kit.PlayGlyph", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
