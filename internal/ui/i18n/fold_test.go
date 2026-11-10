package i18n

import "testing"

func TestFoldDropsCaseAndAccents(t *testing.T) {
	for in, want := range map[string]string{
		"Sigur Rós":      "sigur ros",
		"BJÖRK":          "bjork",
		"Mötley Crüe":    "motley crue",
		"Ólafur Arnalds": "olafur arnalds",
		// Letters Unicode does not decompose.
		"Sørensen": "sorensen",
		"Straße":   "strasse",
		"Łódź":     "lodz",
		"  Édith ": "edith",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}
