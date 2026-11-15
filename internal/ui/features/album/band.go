package album

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// bandAside is the margin column that holds the disc index and the fine
	// print beside the songs, and bandGap the room between it and the songs.
	bandAside = 264
	bandGap   = 48
	// sideShare is how far the band's colour has gone towards its text's
	// opposite at its far end (colours.side).
	sideShare = 0.3
	// dissolveStops is how many colours the dissolve's gradient has: a
	// smootherstep sampled finely enough to show no steps.
	dissolveStops = 24
)

// dissolve is the ground colour laid over the band's foot, whole at its end:
// a gradient of the ground's opacity, built once per theme.
type dissolve struct {
	rev   uint64
	dark  bool
	stops []ui.GradientStop
}

// smootherstep is 6t⁵ − 15t⁴ + 10t³: no slope at either end, so the band
// neither seems to begin to fade abruptly nor to land on the ground.
func smootherstep(t float32) float32 { return t * t * t * (t*(t*6-15) + 10) }

// build makes the gradient for the theme now, unless it is the last's.
func (d *dissolve) build() []ui.GradientStop {
	rev, dark := theme.Revision(), theme.Dark()
	if d.stops != nil && d.rev == rev && d.dark == dark {
		return d.stops
	}
	ground := theme.C("ground")
	d.rev, d.dark = rev, dark
	d.stops = make([]ui.GradientStop, dissolveStops)
	for i := range d.stops {
		t := float32(i) / (dissolveStops - 1)
		d.stops[i] = ui.GradientStop{Color: ground.Alpha(smootherstep(t)), At: t}
	}
	return d.stops
}

// band is the top of the page, in every width: the cover and the head on a
// full-bleed band of the cover's own colour (Apple's background for the cover,
// Go's first dominant colour when Apple gives none) that dissolves into the
// ground below, so the colour has no edge to scroll into, and, wide, deepens
// towards the ground at its far end. Both are gradients of the ground's own
// colour over it, built once per theme, only ever towards the ground, so the
// text's contrast, which the band's colours are chosen for, only improves.
// Reduce effects prints the band solid and ends it square. A cover with no
// colour known keeps the wash of the backdrop under the ground scrim. The
// band's tokens are redeclared (theme.Scope) so everything on it follows it:
// text 4.5:1 on it, whatever the cover.
func (a *album) band(c *ui.Context, m *model, k *colours, lay layout, now time.Time, account bool) {
	fade := lay.fade
	foot := float32(24)
	if lay.stack {
		foot = 16
	}
	top := float32(padTop)
	if lay.compact {
		top = 20
	}
	gap := float32(20)
	if lay.wide {
		gap = 40
		foot = 12
	}
	if theme.ReduceEffects() {
		// No dissolve: the band ends square, with room under what it holds.
		fade, foot = 0, 28
	}
	content := func() {
		a.onBand(k, func() {
			body := ui.Column(c.Key("band-body")).FillWidth()
			body.Children(func() {
				row := ui.Row(c.Key("band-row")).FillWidth().Padding(top, a.gr, 0, a.gl).Gap(gap).AlignItems(ui.Start)
				if lay.wide {
					row.AlignItems(ui.Stretch)
				}
				row.Children(func() {
					a.cover(c, m, lay.cover)
					// Wide, the whole head sits on the cover's bottom edge, as a
					// record's header does; a head taller than the cover starts
					// at the top.
					head := ui.Column(c.Key("head")).Grow(1).Basis(0).MinWidth(0)
					if lay.wide {
						head.Justify(ui.End)
					}
					head.Children(func() { a.head(c, m, k, now, account, !lay.stack, lay, body) })
				})
				if lay.stack {
					ui.Row(c.Key("band-actions")).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Padding(20, a.gr, 0, a.gl).Children(func() {
						a.actions(c, m, account, body)
					})
				}
				// The room under the head, and the dissolve's: nothing sits in it.
				ui.Box(c.Key("room")).FillWidth().Height(foot + fade)
			})
		})
		if fade > 0 {
			if lay.wide && k != nil {
				ui.Box(c.Key("side")).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().
					LinearGradient(ui.LinearGradient{Angle: 90, Stops: k.side})
			}
			ui.Box(c.Key("dissolve")).Absolute().Left(0).Right(0).Bottom(0).Height(fade).PassThrough().
				LinearGradient(ui.LinearGradient{Angle: 180, Stops: a.dissolve.build()})
		}
	}
	if k != nil {
		ui.Box(c.Key("band")).FillWidth().Background(k.surface).Children(content)
		return
	}
	colour := ""
	if len(m.header.Colors) > 0 {
		colour = m.header.Colors[0]
	}
	kit.Backdrop(c.Key("band"), a.env.Art, kit.BackdropOpts{Src: m.header.Backdrop, Color: colour, Tone: kit.ToneGround}, func() {
		ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).PassThrough().Background(theme.C("ink").Alpha(0.04))
		content()
	}).FillWidth()
}

// onBand builds children with the band's tokens, when the cover gives colours.
func (a *album) onBand(k *colours, children func()) {
	if k == nil {
		children()
		return
	}
	theme.Scope(k.bandTok, children)
}

// listing is what lies under the band: the songs and the notes, and the disc
// index and the fine print. From 860 of content they take a margin beside the
// songs (the margin first in the tree, so Tab meets the disc index before the
// songs), held at the pane's top while the songs scroll when the pane shows
// all of it; narrower, the fine print follows the songs.
func (a *album) listing(c *ui.Context, m *model, lay layout, tracks func()) {
	notes := m.notes != "" && m.notes != m.lede
	fine := len(m.facts) > 0 || m.rights != ""
	if !lay.wide {
		ui.Column(c.Key("listing")).FillWidth().Padding(0, a.gr, 0, a.gl).Children(func() {
			ui.Column(c.Key("songs")).FillWidth().Children(tracks)
			if fine {
				ui.Column(c.Key("fine")).Margin(24, 0, 0, 0).Children(func() { a.finePrint(c, m) })
			}
			if notes {
				ui.Column(c.Key("notes")).Children(func() { a.notesShelf(c, m, false) })
			}
		})
		return
	}
	aside := len(m.discs) > 0 || fine
	grid := ui.Grid(c.Key("listing")).FillWidth().Padding(0, a.gr, 0, a.gl).AlignItems(ui.Start).RowTracks(ui.Fr(1))
	if aside {
		grid.ColumnTracks(ui.Fr(1), ui.Fixed(bandGap), ui.Fixed(bandAside))
	} else {
		grid.ColumnTracks(ui.Fr(1))
	}
	grid.Children(func() {
		if aside {
			a.margin(c, m, fine)
		}
		ui.Column(c.Key("songs-col")).ColumnStart(1).RowStart(1).AlignSelf(ui.Start).MinWidth(0).Children(func() {
			ui.Column(c.Key("songs")).MinWidth(0).FillWidth().Children(tracks)
			if notes {
				ui.Column(c.Key("notes")).Margin(40, 0, 0, 0).MinWidth(0).Children(func() { a.notesShelf(c, m, true) })
			}
		})
	})
}

// marginTop is how far under the pane's top the held margin stays.
const marginTop = 16

// margin is the disc index and the fine print, one left edge, beside the
// songs.
func (a *album) margin(c *ui.Context, m *model, fine bool) {
	e := ui.Column(c.Key("margin")).ColumnStart(3).RowStart(1).AlignSelf(ui.Start)
	e.Children(func() {
		if len(m.discs) > 0 {
			ui.Column(c.Key("discs")).Margin(8, 0, 0, 0).Children(func() { a.discIndex(c, m) })
		}
		if fine {
			if len(m.discs) > 0 {
				ui.Box(c.Key("rule")).Margin(20, 0, 0, 0).FillWidth().Height(1).Background(theme.C("line"))
			}
			ui.Column(c.Key("fine")).Margin(20, 0, 0, 0).Children(func() { a.finePrint(c, m) })
		}
	})
	// It holds beside the songs while they scroll when the pane shows all of
	// it (measured from its height in the frame before).
	if b := e.LayoutBounds(); b.H > 0 {
		a.marginH = b.H
	}
	if a.pane == nil {
		return
	}
	if pane := a.pane.Bounds(); pane.H > 0 && a.marginH+marginTop <= pane.H {
		e.Sticky().Top(marginTop)
	}
}
