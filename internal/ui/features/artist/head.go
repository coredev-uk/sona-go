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

// latestRelease is the latest release as the header's slip: its cover, title
// and catalogue line linking to the album, with a play control when the album
// plays. Beside the text (beside) it is a column: the cover with the amber
// play disc over its corner, the lines under it; under the text it is a row:
// the cover and lines, the disc at the end.
func (a *artist) latestRelease(c *ui.Context, al services.Album, size float32, beside bool) {
	tk := theme.Current()
	title, release := releaseTitle(al.Name)
	kind := i18n.T("Album")
	switch {
	case release == "ep":
		kind = i18n.T("EP")
	case release == "single" || al.Single:
		kind = i18n.T("Single")
	}
	when := releaseBadge(al, a.now())
	if when == "" {
		when = i18n.FormatISODate(al.ReleaseDate, i18n.DateMedium)
	}
	whenLine := i18n.T("Latest release")
	if when != "" {
		whenLine += " · " + when
	}
	path := albumPath(al.ID)
	kindLine := catalogueLine(kind, "", al.TrackCount)
	item := services.Item{Kind: services.ItemAlbum, ID: al.ID, Name: al.Name, Subtitle: al.Artist, Art: al.Art, Explicit: al.Explicit, Album: &al}
	canPlay := a.env.Players.CanPlay(item)
	// The link is named by the title; the lines ride in its description, as
	// the one piece MyGo reads the link as holds no more.
	slip := ui.ButtonBase(c.Key("latest")).MinWidth(0).Radius(tk.Radius["sm"]).Label(title).
		Description(whenLine + " · " + kindLine)
	if beside {
		slip.Column().Width(size).Shrink(0).AlignItems(ui.Start).Gap(10)
	} else {
		slip.Row().Grow(1).Basis(0).Justify(ui.Start).AlignItems(ui.Center).Gap(16)
	}
	if a.has(path) {
		slip.Role(ui.RoleLink)
	}
	slip.Children(func() {
		kit.Artwork(c, a.env.Art, al.Art.Medium, size, kit.ArtOpts{Edge: true})
		ui.Column(c.Key("lines")).MinWidth(0).Shrink(1).Gap(2).Children(func() {
			meta := func(s string, lines int) {
				ui.Text(c, s).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
					TextColor(theme.C("ink-muted")).MaxLines(lines).TextAlign(ui.Start)
			}
			if !beside {
				meta(whenLine, 1)
			}
			t := ui.Text(c, title).FontSize(tk.Type["heading"].Size).FixedLineHeight(tk.Type["heading"].LineHeight).
				FontWeight(int(tk.Type["heading"].Weight)).TextColor(theme.C("ink")).MaxLines(2).TextAlign(ui.Start)
			if slip.Hovered() {
				t.Underline()
			}
			if beside {
				meta(whenLine, 2)
			}
			meta(kindLine, 1)
		})
	})
	if slip.Clicked() && a.has(path) {
		a.env.Nav.Push(path)
	}
	if canPlay {
		disc := kit.Button(c.Key("play-latest"), "", kit.ButtonOpts{
			Variant: kit.Play, Size: kit.SizeLG, Icon: "play", Filled: true,
			A11yLabel: i18n.T("Play {label}", "label", title), Tip: i18n.T("Play"),
		})
		if beside {
			disc.Absolute().Left(8).Top(size - 48)
		} else {
			disc.Shrink(0)
		}
		if disc.Clicked() {
			a.env.Players.PlayItem(item)
		}
	}
}

// head is the act's press sheet: the name at 32/36 leading, the catalogue
// line, the biography's opening lines and the actions in one column, the
// latest release as a second column (from slipFrom) and the round portrait
// at the trailing edge, on a wash of the portrait's colour (Go's
// pre-blurred backdrop under the ground scrim). Narrower, the latest release
// is a row under the text. It bleeds to the content edges.
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
		ui.Row(c.Key("row")).FillWidth().AlignItems(ui.Start).Gap(32).Children(func() {
			ui.Column(c.Key("text")).Grow(1).Basis(0).MinWidth(0).Children(func() {
				d := tk.Type["display"]
				ui.Text(c, art.Name).FontSize(d.Size).FixedLineHeight(d.LineHeight).FontWeight(int(d.Weight)).LetterSpacing(d.Tracking).
					TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(1)
				ui.Text(c, p.line).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
					TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
				if p.lead != "" {
					ui.Column(c.Key("lead")).MaxWidth(512).Margin(16, 0, 0, 0).AlignItems(ui.Start).Children(func() {
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
			if p.latest != nil && lay.slipBeside {
				ui.Column(c.Key("slip")).Width(lay.slip).Shrink(0).Children(func() {
					a.latestRelease(c, *p.latest, lay.slip, true)
				})
			}
			// The person stays round, at the trailing edge.
			if p.header.Motion != "" {
				kit.MotionCover(c, a.env.Motion, a.env.Art, art.Art.Large, p.header.Motion, lay.portrait, kit.MotionOpts{VideoSize: 200, Round: true})
			} else {
				kit.Artwork(c, a.env.Art, art.Art.Large, lay.portrait, kit.ArtOpts{Edge: true, Round: true})
			}
		})
		if p.latest != nil && !lay.slipBeside {
			ui.Column(c.Key("slip-row")).FillWidth().Margin(24, 0, 0, 0).Children(func() {
				ui.Box(c.Key("rule")).FillWidth().Height(1).Background(theme.C("line"))
				ui.Row(c.Key("slip")).FillWidth().Margin(16, 0, 0, 0).AlignItems(ui.Center).Gap(8).Children(func() {
					a.latestRelease(c, *p.latest, lay.slip, false)
				})
			})
		}
	})
	e.Column().AlignSelf(ui.Stretch).Padding(32, g+scrollbar, 28, g).Margin(-28, -(g + scrollbar), 32, -g).Shrink(0)
}
