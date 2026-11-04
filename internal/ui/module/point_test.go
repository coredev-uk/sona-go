package module

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPointBuildRangeBuildsOnlyItsOrders(t *testing.T) {
	var p Point
	var got []int
	add := func(order int) { p.Add(order, func(*ui.Context) { got = append(got, order) }) }
	for _, o := range []int{60, 10, 50, 70} {
		add(o)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		got = nil
		p.BuildRange(c, 0, ControlsAfterQueue)
		p.BuildRange(c, ControlsAfterQueue, 1<<30)
	}, 100, 100)
	tt.Frame()
	if want := []int{10, 50, 60, 70}; len(got) != len(want) || got[0] != 10 || got[1] != 50 || got[2] != 60 || got[3] != 70 {
		t.Errorf("built %v, want %v", got, want)
	}
}
