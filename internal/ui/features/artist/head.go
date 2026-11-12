package artist

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// headProps is what the artist's header shows.
type headProps struct {
	artist services.Artist
	header services.Header
	// line is the catalogue line: "Artist · Alternative · Abingdon,
	// Oxfordshire, England".
	line string
	// lead is the biography's first paragraph, with "Read more" to the
	// About band.
	lead    string
	latest  *services.Album
	actions func()
	// width is the content width (layoutFor).
	width float32
}

// latestRelease is the latest release as a slip: its cover, title and
// catalogue line, linking to the album.
func (a *artist) latestRelease(c *ui.Context, al services.Album, size float32) {
	tk := theme.Current()
	title, release := kit.ReleaseTitle(al.Name)
	kind := i18n.T("Album")
	switch {
	case release == "ep":
		kind = i18n.T("EP")
	case release == "single" || al.Single:
		kind = i18n.T("Single")
	}
	when := kit.ReleaseBadge(al, a.now())
	if when == "" {
		when = i18n.FormatISODate(al.ReleaseDate, i18n.DateMedium)
	}
	whenLine := i18n.Meta(i18n.T("Latest release"), when)
	path := albumPath(al.ID)
	kindLine := i18n.CatalogueLine(kind, "", al.TrackCount, 0)
	// The link is named by the title; the lines under it ride in its
	// description, as the one piece MyGo reads the link as holds no more.
	slip := ui.ButtonBase(c.Key("slip")).Row().AlignItems(ui.End).Gap(12).MinWidth(0).Radius(tk.Radius["sm"]).Label(title).
		Description(i18n.Meta(whenLine, kindLine))
	if a.env.Routed(path) {
		slip.Role(ui.RoleLink)
	}
	slip.Children(func() {
		kit.Artwork(c, a.env.Art, al.Art.Medium, size, kit.ArtOpts{Edge: true})
		ui.Column(c).MinWidth(0).Shrink(1).Children(func() {
			t := ui.Text(c, title).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).FontWeight(500).
				TextColor(theme.C("ink")).MaxLines(2).TextAlign(ui.Start)
			if slip.Hovered() {
				t.Underline()
			}
			meta := func(s string) {
				ui.Text(c, s).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
					TextColor(theme.C("ink-muted")).SingleLine().TextAlign(ui.Start)
			}
			meta(whenLine)
			meta(kindLine)
		})
	})
	if slip.Clicked() && a.env.Routed(path) {
		a.env.Nav.Push(path)
	}
}

// head is the artist's file: the round portrait, the name at 32/36, the
// catalogue line, the biography's opening lines and the actions, on a wash
// of the portrait's colour (Go's pre-blurred backdrop under the ground
// scrim). The latest release stands in the header's second column at wide
// widths and under the portrait and text otherwise. It bleeds to the content
// edges.
func (a *artist) head(c *ui.Context, pg *pageState, p headProps) {
	tk := theme.Current()
	g, scrollbar := gutter(c), tk.Metric["scrollbar"]
	lay := layoutFor(p.width)
	art := p.artist
	var colour string
	if len(p.header.Colors) > 0 {
		colour = p.header.Colors[0]
	}
	// The wash is the header's parent: an absolutely placed child draws over
	// its siblings, scrim included.
	e := kit.Backdrop(c.Key("head"), a.env.Art, kit.BackdropOpts{Src: p.header.Backdrop, Color: colour, Tone: kit.ToneGround}, func() {
		ui.Row(c.Key("row")).FillWidth().AlignItems(ui.End).Gap(28).Children(func() {
			if p.header.Motion != "" {
				kit.MotionCover(c, a.env.Motion, a.env.Art, art.Art.Large, p.header.Motion, lay.portrait, kit.MotionOpts{VideoSize: 200, Round: true})
			} else {
				kit.Artwork(c, a.env.Art, art.Art.Large, lay.portrait, kit.ArtOpts{Edge: true, Round: true})
			}
			ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Padding(0, 0, 4, 0).Children(func() {
				theme.StyleText(ui.Text(c, art.Name), "display").
					TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(1)
				ui.Text(c, p.line).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
					TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
				if p.lead != "" {
					ui.Column(c.Key("lead")).MaxWidth(512).Margin(12, 0, 0, 0).AlignItems(ui.Start).Children(func() {
						ui.Text(c, p.lead).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
							TextColor(theme.C("ink-muted")).MaxLines(2)
						more := ui.ButtonBase(c.Key("more")).Margin(2, 0, 0, 0).Radius(tk.Radius["sm"]).Label(i18n.T("Read more"))
						more.Children(func() {
							t := ui.Text(c, i18n.T("Read more")).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
								FontWeight(500).TextColor(theme.C("ink-muted"))
							if more.Hovered() {
								t.TextColor(theme.C("ink")).Underline()
							}
						})
						if more.Clicked() {
							pg.readMore = true
							c.Invalidate()
						}
					})
				}
				if p.actions != nil {
					ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(p.actions)
				}
			})
			if p.latest != nil && lay.wide {
				ui.Column(c.Key("slip")).Width(300).Shrink(0).Padding(0, 0, 4, 0).Children(func() {
					a.latestRelease(c, *p.latest, lay.slip)
				})
			}
		})
		if p.latest != nil && !lay.wide {
			ui.Row(c.Key("slip-row")).Margin(24, 0, 0, 0).Children(func() {
				a.latestRelease(c, *p.latest, lay.slip)
			})
		}
	})
	e.Column().AlignSelf(ui.Stretch).Padding(32, g+scrollbar, 28, g).Margin(-28, -(g + scrollbar), 32, -g).Shrink(0)
}
