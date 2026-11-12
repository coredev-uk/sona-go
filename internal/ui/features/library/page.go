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
	// gridTop, gridW are where the list's body starts in the scroll
	// container's content and how wide it is, as the last frame laid it
	// out: what the letter rail needs to scroll an item to the top.
	gridTop, gridW float32
}

// scrollPage builds a library page's frame: one scroll container holding
// the page's column (the header scrolls with the list), inside the shell's
// gutters. body builds the page into it and is given the Pane virtual lists
// are built in. identity names what the page lists (its sort and type): a
// change starts the list from its top.
func (l *library) scrollPage(c *ui.Context, pv *pageView, identity string, body func(pane *kit.Pane)) {
	l.scrollPageRail(c, pv, identity, false, nil, body)
}

// scrollPageRail is scrollPage with a letter rail standing at the page's
// right edge, outside the scroll container so that it stays while the list
// scrolls: rail builds it. slot keeps the rail's place whether or not there
// is one (a list ordered another way, a gate in place of the list), so the
// page's controls and list never move when the rail comes or goes.
func (l *library) scrollPageRail(c *ui.Context, pv *pageView, identity string, slot bool, rail func(), body func(pane *kit.Pane)) {
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
	right := gutter
	if slot {
		right = 8 // the rail is the rest of the gutter
	}
	devstate.Scroll(c, &pv.pos) // dev builds: the offset survives a restart
	page := func() {
		sc := ui.Scroll(c).TrackScroll(&pv.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
		sc.Children(func() {
			ui.Column(c).Padding(padTop, right+tk.Metric["scrollbar"], padBottom, gutter).Children(func() {
				body(&kit.Pane{Pos: &pv.pos, Bounds: sc.Bounds})
			})
		})
		if b := sc.Bounds(); b.W != pv.width || b.H != pv.height {
			pv.width, pv.height = b.W, b.H
			c.Invalidate() // the narrow gutters follow the width
		}
	}
	if !slot {
		page()
		return
	}
	ui.Row(c.Key("railed")).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		page()
		ui.Column(c.Key("rail-slot")).Shrink(0).Width(max(gutter, railWidth)).Padding(padTop+4, 0, 0, 0).Children(func() {
			if rail != nil {
				rail()
			}
		})
	})
}

// gridAnchor marks where a list's body starts in the page's column: a
// zero-height box, read back after the frame is laid out. A page with a
// letter rail builds it just before the list.
func (pv *pageView) gridAnchor(c *ui.Context, pane *kit.Pane) {
	b := ui.Box(c.Key("anchor")).FillWidth().Height(0).Bounds()
	pv.gridTop, pv.gridW = b.Y-pane.Bounds().Y+pane.Pos.Y, b.W
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
