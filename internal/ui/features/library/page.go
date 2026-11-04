package library

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry, from the reference page (AppShell's main, PageHeader)
// in DIPs.
const (
	padTop    = 28 // pt-7
	padBottom = 48 // pb-12
	narrow    = 960
)

// pageView is the view state every library page keeps: the scroll place and
// the size of its scroll container as laid out in the last frame.
type pageView struct {
	pos ui.ScrollState
	// width, height are the scroll container's size, zero until a frame
	// laid it out.
	width, height float32
	// shown is the identity of the list showing: another list starts from
	// its top.
	shown string
}

// scrollPage builds a library page's frame: one scroll container holding
// the page's column (the header scrolls with the list), inside the shell's
// gutters. body builds the page into it and is given the Pane virtual lists
// are built in. identity names what the page lists (its sort and type): a
// change starts the list from its top.
func (l *library) scrollPage(c *ui.Context, pv *pageView, identity string, body func(pane *kit.Pane)) {
	if pv.shown != identity {
		if pv.shown != "" {
			pv.pos = ui.ScrollState{}
		}
		pv.shown = identity
	}
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if pv.width > 0 && pv.width < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	devstate.Scroll(c, &pv.pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&pv.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+tk.Metric["scrollbar"], padBottom, gutter).Children(func() {
			body(&kit.Pane{Pos: &pv.pos, Bounds: sc.Bounds})
		})
	})
	if b := sc.Bounds(); b.W != pv.width || b.H != pv.height {
		pv.width, pv.height = b.W, b.H
		c.Invalidate() // the narrow gutters follow the width
	}
}

// sortSelect is the Sort select of a list's header: it reports the new sort
// (and its natural order) when the user chose another.
func (l *library) sortSelect(c *ui.Context, spec listSpec, q listQuery) {
	items := make([]kit.SelectItem, len(spec.sorts))
	for i, o := range spec.sorts {
		items[i] = kit.SelectOption(o.key, o.label())
	}
	value := q.sort
	if kit.Select(c, &value, items, kit.SelectOpts{Small: true, Width: 160, A11yLabel: i18n.T("Sort")}).Changed() && value != q.sort {
		next := q
		next.sort = value
		next.desc = startsDesc(value)
		l.env.Nav.Replace(spec.location(next))
	}
}
