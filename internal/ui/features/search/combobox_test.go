package search

import "testing"

// v0.2.0's combobox.test.ts: moveActive and splitMatch.

func TestMoveActiveEntersTheListFromTheFieldAtEitherEnd(t *testing.T) {
	if got := moveActive(-1, 1, 4); got != 0 {
		t.Errorf("down from the field = %d, want 0", got)
	}
	if got := moveActive(-1, -1, 4); got != 3 {
		t.Errorf("up from the field = %d, want 3", got)
	}
}

func TestMoveActiveMovesWithinTheListAndReturnsToTheFieldPastTheEnds(t *testing.T) {
	for _, c := range []struct{ active, delta, count, want int }{
		{1, 1, 4, 2},
		{2, -1, 4, 1},
		{3, 1, 4, -1},
		{0, -1, 4, -1},
	} {
		if got := moveActive(c.active, c.delta, c.count); got != c.want {
			t.Errorf("moveActive(%d, %d, %d) = %d, want %d", c.active, c.delta, c.count, got, c.want)
		}
	}
}

func TestMoveActiveStaysInTheFieldWithNothingToMoveTo(t *testing.T) {
	if got := moveActive(-1, 1, 0); got != -1 {
		t.Errorf("down with no options = %d, want -1", got)
	}
	if got := moveActive(2, -1, 0); got != -1 {
		t.Errorf("up with no options = %d, want -1", got)
	}
}

func TestSplitMatch(t *testing.T) {
	for _, c := range []struct {
		name, text, typed, before, match, after string
	}{
		{"ignores case and outer spaces", "taylor swift", " Tay", "", "tay", "lor swift"},
		{"finds it inside", "the taylor show", "taylor", "the ", "taylor", " show"},
		{"keeps the text's own case", "Taylor Swift", "TAYLOR", "", "Taylor", " Swift"},
		{"whole text when nothing matches", "blue", "red", "", "", "blue"},
		{"whole text when only spaces are typed", "blue", "  ", "", "", "blue"},
		{"whole text when nothing is typed", "blue", "", "", "", "blue"},
		{"typed longer than the text", "blue", "blue band", "", "", "blue"},
		{"the whole text typed", "blue", "blue", "", "blue", ""},
		{"multi-byte text", "Beyoncé Knowles", "é k", "Beyonc", "é K", "nowles"},
		{"a match at the very end", "ab ÉÉ", "éé", "ab ", "ÉÉ", ""},
	} {
		before, match, after := splitMatch(c.text, c.typed)
		if before != c.before || match != c.match || after != c.after {
			t.Errorf("%s: splitMatch(%q, %q) = [%q %q %q], want [%q %q %q]",
				c.name, c.text, c.typed, before, match, after, c.before, c.match, c.after)
		}
	}
}
