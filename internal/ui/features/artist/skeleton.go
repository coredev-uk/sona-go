package artist

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// skeleton is ArtistSkeleton: the page's frame while it loads, at the final
// sizes: the press sheet (name, catalogue line, two lines of biography, the
// actions, the latest release, the portrait at the trailing edge), then top
// songs and the essential albums panel as loading rows of the real lists.
// Nothing moves when the data lands.
func (a *artist) skeleton(c *ui.Context, width float32) {
	tk := theme.Current()
	hover := theme.C("hover")
	g, scrollbar := gutter(c), tk.Metric["scrollbar"]
	lay := layoutFor(width)
	r := tk.Radius["sm"]
	pct := func(p, h float32) ui.Element { return ui.Box(c).WidthPercent(p).Height(h).Radius(r).Background(hover) }
	ui.Column(c.Key("loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Column(c.Key("head")).AlignSelf(ui.Stretch).Padding(32, g+scrollbar, 28, g).
			Margin(-28, -(g + scrollbar), 32, -g).Shrink(0).Children(func() {
			ui.Row(c.Key("row")).FillWidth().AlignItems(ui.Start).Gap(32).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					ui.Row(c).Height(36).AlignItems(ui.Center).Children(func() { pct(40, 28) })
					ui.Row(c).Height(16).Margin(4, 0, 0, 0).AlignItems(ui.Center).Children(func() { pct(25, 12) })
					ui.Column(c).MaxWidth(512).Margin(16, 0, 0, 0).Children(func() {
						ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { pct(100, 12) })
						ui.Row(c).Height(18).AlignItems(ui.Center).Children(func() { pct(80, 12) })
						ui.Row(c).Height(18).Margin(2, 0, 0, 0).AlignItems(ui.Center).Children(func() { kit.Bar(c, 64, 12) })
					})
					ui.Row(c).Gap(8).AlignItems(ui.Center).Margin(20, 0, 0, 0).Children(func() {
						ui.Box(c).Size(40, 40).Shrink(0).Radius(tk.Radius["full"]).Background(hover)
						kit.Bar(c, 96, 32)
						for range 3 {
							kit.Bar(c, 32, 32)
						}
					})
				})
				if lay.slipBeside {
					ui.Column(c.Key("slip")).Width(lay.slip).Shrink(0).Gap(10).Children(func() {
						ui.Box(c).Size(lay.slip, lay.slip).Shrink(0).Radius(r).Background(hover)
						ui.Column(c).Gap(2).Children(func() {
							ui.Row(c).Height(22).AlignItems(ui.Center).Children(func() { pct(90, 14) })
							ui.Row(c).Height(16).AlignItems(ui.Center).Children(func() { pct(70, 12) })
							ui.Row(c).Height(16).AlignItems(ui.Center).Children(func() { pct(40, 12) })
						})
					})
				}
				ui.Box(c).Size(lay.portrait, lay.portrait).Shrink(0).Radius(tk.Radius["full"]).Background(hover)
			})
			if !lay.slipBeside {
				ui.Column(c.Key("slip-row")).FillWidth().Margin(24, 0, 0, 0).Children(func() {
					ui.Box(c).FillWidth().Height(1).Background(theme.C("line"))
					ui.Row(c).FillWidth().Margin(16, 0, 0, 0).AlignItems(ui.Center).Gap(16).Children(func() {
						ui.Box(c).Size(lay.slip, lay.slip).Shrink(0).Radius(r).Background(hover)
						ui.Column(c).Width(200).Gap(2).Children(func() {
							ui.Row(c).Height(16).AlignItems(ui.Center).Children(func() { pct(70, 12) })
							ui.Row(c).Height(22).AlignItems(ui.Center).Children(func() { pct(100, 14) })
							ui.Row(c).Height(16).AlignItems(ui.Center).Children(func() { pct(40, 12) })
						})
					})
				})
			}
		})
		if width <= 0 {
			return
		}
		heading := func(w float32) {
			ui.Row(c).Height(28).Margin(0, 0, 12, 0).AlignItems(ui.Center).Children(func() { kit.Bar(c, w, 16) })
		}
		tracks := func() {
			heading(96)
			kit.TrackList(c, &a.page.top.TrackListState, topRows, func(int) *services.Song { return nil }, kit.TrackListOpts{
				Columns: kit.TrackColumns{RowNumbers: true, Artwork: true, Album: true}, Art: a.env.Art,
			})
		}
		essential := func() {
			ui.Column(c.Key("panel")).Padding(16, 16, 12, 16).Radius(tk.Radius["lg"]).Background(theme.C("surface")).Children(func() {
				heading(128)
				a.releaseList(c, a.page.list("skeleton"), make([]*services.Album, essentialRows), releaseOpts{columns: map[bool]int{true: 1, false: 2}[lay.wide]})
			})
		}
		if lay.wide {
			ui.Row(c.Key("lead")).FillWidth().Gap(40).AlignItems(ui.Start).Children(func() {
				ui.Column(c.Key("top")).Grow(1).Basis(0).MinWidth(0).Padding(16, 0, 0, 0).Children(tracks)
				ui.Column(c.Key("ess")).Width(leadSide).Shrink(0).Children(essential)
			})
			return
		}
		ui.Column(c.Key("lead")).FillWidth().Children(func() {
			tracks()
			ui.Column(c.Key("ess")).Margin(40, 0, 0, 0).Children(essential)
		})
	})
}
