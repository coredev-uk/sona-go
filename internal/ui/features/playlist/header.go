package playlist

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The header's geometry, in DIPs: the cover is the card's front panel, 224
// square (144 when the window is under kit.PageNarrow, 288 from headLargeFrom),
// and the text beside it is as tall as the cover, the title and byline at its
// top and the actions on its bottom edge.
const (
	headCover       = 224
	headCoverNarrow = 144
	headCoverLarge  = 288
	// headLargeFrom is the window width from which the cover takes its
	// large class, so a wide window's header is not a small cover in a
	// wide wash.
	headLargeFrom = 1600
	headBandH     = 40
)

// headArt is the cover's size at a window width: one of three fixed classes,
// so Go serves the exact box.
func headArt(ww float32) float32 {
	switch {
	case ww < kit.PageNarrow:
		return headCoverNarrow
	case ww >= headLargeFrom:
		return headCoverLarge
	}
	return headCover
}

// ledgerCell is one fact of the ledger strip: the text and, for "Updated …",
// the exact date and time behind it (the cell is then a tab stop with a
// tooltip).
type ledgerCell struct {
	text, detail string
	// lead marks the cell that names the kind of playlist: ink, not muted.
	lead bool
}

// ledger is the strip under the cover: the kind of playlist, how many songs,
// the running time, and when it last changed. Cells with nothing to say are
// left out.
func (p *playlists) ledger(d services.PlaylistDetail, collab *services.PlaylistCollaboration, note, detail string) []ledgerCell {
	kind := i18n.T("Playlist")
	switch {
	case collab != nil:
		kind = i18n.T("Collaborative playlist")
	case d.Kind == "personal-mix":
		kind = i18n.T("Made for you")
	case d.IsChart:
		kind = i18n.T("Chart")
	case d.Favorited:
		kind = i18n.T("Your favourites")
	}
	cells := []ledgerCell{{text: kind, lead: true}}
	if collab != nil && collab.Viewer == services.RoleCollaborator {
		cells = append(cells, ledgerCell{text: i18n.T("You're a collaborator")})
	}
	if n := len(d.Tracks); n > 0 {
		cells = append(cells, ledgerCell{text: i18n.T("{0, plural, one {# song} other {# songs}}", "0", n)})
	}
	if d.DurationMs > 0 {
		cells = append(cells, ledgerCell{text: i18n.FormatRunningTime(time.Duration(d.DurationMs) * time.Millisecond)})
	}
	if note != "" {
		cells = append(cells, ledgerCell{text: note, detail: detail})
	}
	return cells
}

// ledgerStrip draws the cells in one row, a hairline between each.
func ledgerStrip(c *ui.Context, cells []ledgerCell) {
	tk := theme.Current()
	sub := tk.Type["sub"]
	ui.Row(c.Key("ledger")).Grow(1).MinWidth(0).Wrap().AlignItems(ui.Center).Gap(12).Children(func() {
		for i, cell := range cells {
			if i > 0 {
				ui.Box(c.Key("rule-" + strconv.Itoa(i))).Width(1).Height(14).Shrink(0).Background(theme.C("control-line")).Role(ui.RoleNone)
			}
			col := theme.C("ink-muted")
			if cell.lead {
				col = theme.C("ink")
			}
			e := ui.Text(c.Key("cell-"+strconv.Itoa(i)), cell.text).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
				TextColor(col).FontFeatures("tnum").SingleLine().Shrink(0)
			if cell.lead {
				e.FontWeight(600)
			}
			if cell.detail != "" {
				e.PaddingX(2).Radius(tk.Radius["sm"]).TabStop(true)
				kit.Tooltip(c, e, cell.detail)
			}
		}
	})
}

// header is the page's head: on a wash of the cover's colour, the cover
// beside the title, byline and description with the actions along the
// cover's bottom edge, and under both the ledger strip, with the song
// finder (find, when the page has one) at its right end. It bleeds to the
// content edges by the page's gutter.
func (p *playlists) header(c *ui.Context, d services.PlaylistDetail, byline func(), cells []ledgerCell, actions, find func()) {
	tk := theme.Current()
	ww, _ := c.Size()
	gutter, art := tk.Metric["gutter"], headArt(ww)
	if ww < kit.PageNarrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	pl, hd := d.Playlist, d.Header
	var colour string
	if len(hd.Colors) > 0 {
		colour = hd.Colors[0]
	}
	body := func() {
		ui.Row(c.Key("row")).FillWidth().Gap(32).AlignItems(ui.Stretch).Children(func() {
			if hd.Motion != "" {
				kit.MotionCover(c, p.env.Motion, p.env.Art, pl.Art.Large, hd.Motion, art, kit.MotionOpts{VideoSize: headCoverLarge})
			} else {
				kit.Artwork(c, p.env.Art, pl.Art.Large, art, kit.ArtOpts{Edge: true})
			}
			ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Justify(ui.SpaceBetween).Gap(16).Children(func() {
				ui.Column(c.Key("titles")).MinWidth(0).Children(func() {
					t := tk.Type["display"]
					ui.Text(c, pl.Name).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking).
						TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(1)
					if byline != nil {
						ui.Row(c.Key("byline")).Margin(6, 0, 0, 0).MinWidth(0).TextColor(theme.C("ink")).FontSize(tk.Type["heading"].Size).
							FontWeight(500).Children(byline)
					}
					if pl.Description != "" {
						s := tk.Type["secondary"]
						ui.Text(c, pl.Description).FontSize(s.Size).FixedLineHeight(s.LineHeight).
							TextColor(theme.C("ink-muted")).MaxLines(2).MaxWidth(560).Margin(10, 0, 0, 0)
					}
				})
				ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Children(actions)
			})
		})
		ui.Row(c.Key("band")).FillWidth().MinHeight(headBandH).Margin(24, 0, 0, 0).Gap(16).Wrap().AlignItems(ui.Center).Children(func() {
			ledgerStrip(c, cells)
			if find != nil {
				find()
			}
		})
	}
	kit.Backdrop(c.Key("wash"), p.env.Art, kit.BackdropOpts{Src: hd.Backdrop, Color: colour, Tone: kit.ToneGround}, func() {
		ui.Column(c).FillWidth().Padding(32, gutter, 4, gutter).Children(body)
	}).AlignSelf(ui.Stretch).Shrink(0).Margin(-28, -gutter, 4, -gutter)
}

// pageSkeleton is the loading page at the real layout: the cover, the title,
// the byline and the actions, the ledger strip, and the rows of the list.
func (p *playlists) pageSkeleton(c *ui.Context) {
	tk := theme.Current()
	ww, _ := c.Size()
	art := headArt(ww)
	hover := theme.C("hover")
	r := tk.Radius["sm"]
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Shrink(0).Radius(r).Background(hover)
	}
	ui.Column(c).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Row(c.Key("row")).FillWidth().Gap(32).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Size(art, art).Shrink(0).Radius(r).Background(hover)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Justify(ui.SpaceBetween).Children(func() {
				ui.Column(c).Children(func() {
					ui.Box(c).Height(32).WidthPercent(40).Radius(r).Background(hover)
					ui.Box(c).Height(16).WidthPercent(22).Radius(r).Background(hover).Margin(14, 0, 0, 0)
				})
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(40, 40).Radius(tk.Radius["full"]).Background(hover)
					ui.Box(c).Size(96, 32).Radius(tk.Radius["md"]).Background(hover)
				})
			})
		})
		ui.Row(c.Key("band")).Height(headBandH).Margin(24, 0, 12, 0).Gap(12).AlignItems(ui.Center).Children(func() {
			for _, w := range []float32{72, 56, 56, 112} {
				bar(w, 14)
			}
		})
		for i := range 8 {
			ui.Row(c.Key("skel-" + strconv.Itoa(i))).FillWidth().Height(kit.TrackRowHeight).PaddingX(8).Gap(16).AlignItems(ui.Center).Children(func() {
				bar(20, 12)
				ui.Box(c).Height(14).WidthPercent(float32(30 + (i*17)%30)).Radius(r).Background(hover)
			})
		}
	})
}

// emptyBlock is a list with nothing in it: a title and a sentence, flush
// with the page's left edge.
func emptyBlock(c *ui.Context, title, text string) {
	ui.Column(c.Key("empty")).FillWidth().Gap(4).Padding(12, 0, 8, 0).Children(func() {
		theme.Text(c, title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2)
		theme.Text(c, text, "body").TextColor(theme.C("ink-muted")).MaxWidth(480)
	})
}
