package namefold

import "testing"

func TestFold(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"Beyoncé", "BEYONCE"},
		{"Sigur Rós", "sigur ros"},
		{"Ed O’Brien", "ed o'brien"},
		{"ＡＢＣ", "abc"},
		{"  Zara   Larsson ", "zara larsson"},
	} {
		if Fold(c.a) != Fold(c.b) {
			t.Errorf("Fold(%q) = %q, Fold(%q) = %q: want equal", c.a, Fold(c.a), c.b, Fold(c.b))
		}
	}
	if Fold("Alesso") == Fold("Alesso & Zara Larsson") {
		t.Error("a combined name folds like its first artist")
	}
}
