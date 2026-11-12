package library

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// railFrame is the view state of a page with a letter rail: the shared page
// frame, and where the list's body starts in the scroll container's content
// and how wide it is, as the last frame laid it out: what the rail needs to
// scroll an item to the top.
type railFrame struct {
	kit.PageFrame
	gridTop, gridW float32
}

// scrollRail is PageFrame.Scroll with a letter rail standing at the page's
// right edge, outside the scroll container so that it stays while the list
// scrolls: rail builds it. The rail keeps its place whether or not there is
// one (a list ordered another way, a gate in place of the list), so the
// page's controls and list never move when the rail comes or goes: the rail
// is the rest of the right gutter.
func (l *library) scrollRail(c *ui.Context, f *railFrame, identity string, rail func(), body func(pane *kit.Pane)) {
	if f.Shown != identity {
		if f.Shown != "" {
			f.Pos = ui.ScrollState{}
		}
		f.Shown = identity
	}
	gutter := f.Gutter()
	ui.Row(c.Key("railed")).Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
		devstate.Scroll(c, &f.Pos) // dev builds: the offset survives a restart
		sc := ui.Scroll(c).TrackScroll(&f.Pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
		sc.Children(func() {
			ui.Column(c).Padding(kit.PagePadTop, 8+theme.Current().Metric["scrollbar"], kit.PagePadBottom, gutter).Children(func() {
				body(&kit.Pane{Pos: &f.Pos, Bounds: sc.Bounds})
			})
		})
		if b := sc.Bounds(); b.W != f.Width || b.H != f.Height || b.Y != f.Top {
			f.Width, f.Height, f.Top = b.W, b.H, b.Y
			c.Invalidate() // the narrow gutters follow the width
		}
		ui.Column(c.Key("rail-slot")).Shrink(0).Width(max(gutter, railWidth)).Padding(kit.PagePadTop+4, 0, 0, 0).Children(func() {
			if rail != nil {
				rail()
			}
		})
	})
}

// gridAnchor marks where a list's body starts in the page's column: a
// zero-height box, read back after the frame is laid out. A page with a
// letter rail builds it just before the list.
func (f *railFrame) gridAnchor(c *ui.Context, pane *kit.Pane) {
	b := ui.Box(c.Key("anchor")).FillWidth().Height(0).Bounds()
	f.gridTop, f.gridW = b.Y-pane.Bounds().Y+pane.Pos.Y, b.W
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
