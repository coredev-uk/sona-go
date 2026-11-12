package search

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The browse skeleton shows browseSkeleton placeholders whatever the width:
// a row count that rounded down dropped the remainder.
func TestBrowseSkeletonHasTwelvePlaceholders(t *testing.T) {
	for _, width := range []float32{640, 1000} { // 3 and 5 columns of 160
		cols, _ := kit.CoverColumns(width, 160)
		total := 0
		for _, n := range skeletonRows(browseSkeleton, cols) {
			if n < 1 || n > cols {
				t.Fatalf("width %v: a row of %d in %d columns", width, n, cols)
			}
			total += n
		}
		if total != browseSkeleton {
			t.Errorf("width %v (%d columns): %d placeholders, want %d", width, cols, total, browseSkeleton)
		}
	}
	if cols, _ := kit.CoverColumns(1000, 160); cols != 5 {
		t.Fatalf("1000 DIPs hold %d columns, the test means 5", cols)
	}
}
