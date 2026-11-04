package kit

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// BlockTop is the offset, in its pane's content, of the block of row i: the
// disc and work headings before it, then the row (v0.2.0's album disc index
// measured the heading's wrapper). It is where the last frame built the
// list; rows of a virtual list have no headings.
func (st *TrackListState) BlockTop(o *TrackListOpts, count int, at func(i int) *services.Song, i int) float32 {
	multiDisc := o.Discs && st.multiDisc(count, at)
	y := st.top + float32(i)*TrackRowHeight
	for j := 0; j < i; j++ {
		y += st.headingHeight(o, j, at, multiDisc)
	}
	return y
}

// headingHeight is the height of the headings before row i, as headings
// builds them.
func (st *TrackListState) headingHeight(o *TrackListOpts, i int, at func(i int) *services.Song, multiDisc bool) float32 {
	s := at(i)
	if s == nil {
		return 0
	}
	tk := theme.Current()
	var prev *services.Song
	if i > 0 {
		prev = at(i - 1)
	}
	var h float32
	disc := multiDisc && (i == 0 || prev == nil || prev.DiscNumber != s.DiscNumber)
	if disc {
		top := float32(20)
		if i == 0 {
			top = 8
		}
		h += top + tk.Type["sub"].LineHeight + 8
	}
	if o.Heading != nil && o.Heading(s, prev) != "" {
		top := float32(20)
		if i == 0 || disc {
			top = 8
		}
		h += top + tk.Type["ui"].LineHeight + 6
	}
	return h
}

// ScrollToBlock scrolls the pane so the block of row i (its headings, then
// the row) starts margin DIPs below the pane's top, and gives the row the
// keyboard focus: choosing a disc in an album's index.
func (st *TrackListState) ScrollToBlock(p *Pane, o *TrackListOpts, count int, at func(i int) *services.Song, i int, margin float32) {
	if p == nil || i < 0 || i >= count {
		return
	}
	p.Pos.Y = max(0, st.BlockTop(o, count, at, i)-margin)
	st.nav.SetFocus(i)
	st.nav.Want(i)
}
