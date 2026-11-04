package concerts

import "testing"

func TestMoveFrom(t *testing.T) {
	// Three rows: a link and tickets, a link alone (no seller), a link and tickets.
	counts := []int{2, 1, 2}
	for _, tc := range []struct {
		name string
		key  rovingKey
		at   position
		want position
	}{
		{"down keeps the column", keyDown, position{0, 1}, position{1, 0}}, // clamped to the short row
		{"down again", keyDown, position{1, 0}, position{2, 0}},
		{"up clamps to a short row", keyUp, position{2, 1}, position{1, 0}},
		{"up stops at the first row", keyUp, position{0, 0}, position{0, 0}},
		{"down stops at the last row", keyDown, position{2, 1}, position{2, 1}},
		{"right along a row", keyRight, position{0, 0}, position{0, 1}},
		{"right stops at the row's end", keyRight, position{0, 1}, position{0, 1}},
		{"left along a row", keyLeft, position{2, 1}, position{2, 0}},
		{"right in a one-control row", keyRight, position{1, 0}, position{1, 0}},
		{"home is the first row", keyHome, position{2, 1}, position{0, 1}},
		{"end is the last row", keyEnd, position{0, 1}, position{2, 1}},
	} {
		got, ok := moveFrom(tc.key, tc.at, counts)
		if !ok || got != tc.want {
			t.Errorf("%s: moveFrom = %+v, %v; want %+v", tc.name, got, ok, tc.want)
		}
	}
	if got, ok := moveFrom(keyEnd, position{0, 1}, []int{2, 1}); !ok || got != (position{1, 0}) {
		t.Errorf("end over two rows = %+v", got)
	}
	if _, ok := moveFrom(keyOther, position{0, 0}, counts); ok {
		t.Error("another key moved")
	}
}
