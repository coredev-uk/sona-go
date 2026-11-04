package artist

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// skeleton is ArtistSkeleton: the page's frame while it loads, at the final
// sizes: the header (portrait, name, catalogue line, two lines of biography,
// the actions, the latest release), then top songs and the essential albums
// as loading rows of the real lists. Nothing moves when the data lands.
func (a *artist) skeleton(c *ui.Context, width float32) {
	tk := theme.Current()
	hover := theme.C("hover")
	g, scrollbar := gutter(c), tk.Metric["scrollbar"]
	lay := layoutFor(width)
	r := tk.Radius["sm"]
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Radius(r).Background(hover).Shrink(0)
	}
	pct := func(p, h float32) ui.Element { return ui.Box(c).WidthPercent(p).Height(h).Radius(r).Background(hover) }
	slip := func() {
		ui.Row(c.Key("slip")).AlignItems(ui.End).Gap(12).Children(func() {
			ui.Box(c).Size(lay.slip, lay.slip).Shrink(0).Radius(r).Background(hover)
			ui.Column(c).Width(160).Gap(6).Children(func() {
				pct(100, 14)
				pct(75, 12)
				pct(50, 12)
			})
		})
	}
	ui.Column(c.Key("loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Column(c.Key("head")).AlignSelf(ui.Stretch).Padding(32, g+scrollbar, 28, g).
			Margin(-28, -(g + scrollbar), 32, -g).Shrink(0).Children(func() {
			ui.Row(c.Key("row")).FillWidth().AlignItems(ui.End).Gap(28).Children(func() {
				ui.Box(c).Size(lay.portrait, lay.portrait).Shrink(0).Radius(tk.Radius["full"]).Background(hover)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(0, 0, 4, 0).Children(func() {
					ui.Row(c).Height(36).AlignItems(ui.Center).Children(func() { pct(40, 28) })
					ui.Row(c).Height(16).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() { pct(25, 12) })
					ui.Column(c).MaxWidth(512).Margin(12, 0, 0, 0).Children(func() {
						ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { pct(100, 12) })
						ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { pct(80, 12) })
						ui.Row(c).Height(18).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() { bar(64, 12) })
					})
					ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(func() {
						ui.Box(c).Size(40, 40).Shrink(0).Radius(tk.Radius["full"]).Background(hover)
						bar(96, 32)
						for range 3 {
							bar(32, 32)
						}
					})
				})
				if lay.wide {
					ui.Column(c.Key("wide-slip")).Width(300).Shrink(0).Padding(0, 0, 4, 0).Children(slip)
				}
			})
			if !lay.wide {
				ui.Column(c.Key("slip-row")).Margin(24, 0, 0, 0).Children(slip)
			}
		})
		if width <= 0 {
			return
		}
		heading := func(w float32) {
			ui.Row(c).Height(28).Margin(0, 0, 12, 0).AlignItems(ui.Center).Children(func() { bar(w, 16) })
		}
		tracks := func() {
			heading(96)
			kit.TrackList(c, &a.page.top.TrackListState, topRows, func(int) *services.Song { return nil }, kit.TrackListOpts{
				Columns: kit.TrackColumns{RowNumbers: true, Artwork: true, Album: true}, Art: a.env.Art,
			})
		}
		essential := func() {
			heading(128)
			a.releaseList(c, a.page.list("skeleton"), make([]*services.Album, essentialRows), releaseOpts{columns: map[bool]int{true: 1, false: 2}[lay.wide]})
		}
		if lay.wide {
			ui.Row(c.Key("lead")).FillWidth().Gap(40).AlignItems(ui.Start).Children(func() {
				ui.Column(c.Key("top")).Grow(1).Basis(0).MinWidth(0).Children(tracks)
				ui.Column(c.Key("ess")).Width(300).Shrink(0).Children(essential)
			})
			return
		}
		ui.Column(c.Key("lead")).FillWidth().Children(func() {
			tracks()
			ui.Column(c.Key("ess")).Margin(40, 0, 0, 0).Children(essential)
		})
	})
}
