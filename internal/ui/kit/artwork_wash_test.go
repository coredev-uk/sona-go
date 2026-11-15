package kit

import "testing"

// A small cover (the sidebar's 20 DIP shuffle button) takes a stronger wash
// than a row's cover, and both are the ground colour: the one rule in
// docs/design.md ("Artwork"), not a literal per call site.
func TestArtworkWashIsStrongerOnASmallCover(t *testing.T) {
	row, small := ArtworkWash(false), ArtworkWash(true)
	if small.A <= row.A {
		t.Errorf("small cover alpha %d, row alpha %d: the small cover needs the stronger wash", small.A, row.A)
	}
	if row.A < 170 || row.A > 185 {
		t.Errorf("row wash alpha %d, want about 70%% (178)", row.A)
	}
	if small.A < 210 || small.A > 220 {
		t.Errorf("small wash alpha %d, want about 85%% (217)", small.A)
	}
	if row.R != small.R || row.G != small.G || row.B != small.B {
		t.Errorf("the two washes are different colours: %v and %v", row, small)
	}
}
