package discover

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// topSongs is how many top songs the overview lists before "See all".
const topSongs = 10

// chartsPage is the state of the Charts overview.
type chartsPage struct {
	pg     pageView
	menus  *kit.ActionMenus
	skel   measure
	shelf  map[string]*kit.CoverShelfState
	ranks  rankState
	songs  []services.Song
}

// chartPage is the state of one chart in full.
type chartPage struct {
	pg    pageView
	menus *kit.ActionMenus
	list  listState
	skel  measure
}

// genreChoice is the Genre select: a chart genre, "All genres" for the
// catch-all (Go names it in English, Apple's genre names come localised). It
// is shown only with two genres at least; change is told the new genre ID.
func (d *discover) genreChoice(c *ui.Context, genres []services.Genre, value string, change func(genre string)) {
	if len(genres) < 2 {
		return
	}
	items := make([]kit.SelectItem, len(genres))
	for i, g := range genres {
		name := g.Name
		if g.ID == "" {
			name = i18n.T("All genres")
		}
		items[i] = kit.SelectOption(sortToken(g.ID), name)
	}
	v := sortToken(value)
	sel := kit.Select(c.Key("genre"), &v, items, kit.SelectOpts{Small: true, A11yLabel: i18n.T("Genre"), Width: 192})
	if sel.Changed() && v != sortToken(value) {
		change(sortFromToken(v))
	}
}

// chartsPage is Charts: Apple Music's most played, overall or by genre, plus
// city and country charts.
func (d *discover) chartsPage(c *ui.Context, r *ui.Route) {
	st := &d.charts
	genre := chartsSearch(r)
	q := d.chartsFor(genre)
	st.pg.body(c, "charts|"+genre, func(_ *kit.Pane, _ float32) {
		var actions func()
		sub := ""
		if q.HasData {
			sub = freshness(q.Data.FetchedAt, q.Data.Stale, d.now())
			if len(q.Data.Genres) >= 2 {
				actions = func() {
					d.genreChoice(c, q.Data.Genres, genre, func(g string) { d.env.Nav.Replace(chartsURL(g)) })
				}
			}
		}
		d.masthead(c, i18n.T("Charts"), sub, !q.HasData && q.Err == nil, actions)
		switch {
		case q.HasData:
			d.chartList(c, q.Data)
		case q.Err != nil:
			d.pageError(c, i18n.T("Couldn't load the charts"), q.Err, q.Fetching, d.retry(keyCharts(genre)))
		default:
			d.chartsSkeleton(c, &st.skel, st.pg.width >= rankAlbumAt)
		}
	})
}

// chartList is the overview's charts, in Apple's order: the top songs ranked,
// the others as shelves with "See all" to the chart in full.
func (d *discover) chartList(c *ui.Context, page services.ChartsPage) {
	st := &d.charts
	ctx := d.env.ItemsCtx(st.menus)
	ctx.NoPopup = true // the page draws the menus' popup once, after the charts
	if st.shelf == nil {
		st.shelf = map[string]*kit.CoverShelfState{}
	}
	for i, ch := range page.Charts {
		more := chartURL(ch.Key, page.Genre, ch.ChartID)
		if ch.Key == services.ChartSongs {
			d.topSongs(c, ch, more, ctx)
			continue
		}
		id := ch.Key + ":" + ch.ChartID
		sh := st.shelf[id]
		if sh == nil {
			sh = new(kit.CoverShelfState)
			st.shelf[id] = sh
		}
		minTile := float32(160)
		if ch.Key == services.ChartMusicVideos {
			minTile = 240
		}
		kit.ItemShelf(c, sh, chartTitle(ch), ch.Items, ctx, kit.ItemShelfOpts{First: i == 0, More: more, Min: minTile})
	}
	st.menus.Popup(c)
}

// topSongs is the top songs, ranked, with Play for the whole chart's first
// page.
func (d *discover) topSongs(c *ui.Context, ch services.Chart, more string, ctx kit.ItemsCtx) {
	st := &d.charts
	title := chartTitle(ch)
	st.songs = st.songs[:0]
	for _, it := range ch.Items {
		if it.Song != nil {
			st.songs = append(st.songs, *it.Song)
		}
	}
	songs := st.songs
	ui.Column(c.Key("top-songs")).FillWidth().Margin(8, 0, 40, 0).Role(ui.RoleGroup).Label(title).Children(func() {
		tk := theme.Current()
		ui.Row(c.Key("heading")).FillWidth().MinHeight(32).Margin(0, 0, 12, 0).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			h := tk.Type["heading"]
			ui.Text(c, title).FontSize(h.Size).FixedLineHeight(h.LineHeight).FontWeight(int(h.Weight)).TextColor(theme.C("ink")).
				SingleLine().Role(ui.RoleHeading).Level(2)
			ui.Row(c.Key("play")).Gap(8).AlignItems(ui.Center).Children(func() { d.playButtons(c, title, songs) })
			ui.Box(c.Key("fill")).Grow(1)
			seeAllLink(c, "see-all", func() { d.env.Nav.Push(more) })
		})
		shown := songs[:min(len(songs), topSongs)]
		d.rankList(c, &st.ranks, rankOpts{
			label: title, album: st.pg.width >= rankAlbumAt, count: len(shown),
			songs:  func(i int) *services.Song { return &shown[i] },
			onPlay: func(i int) { d.env.Queue.PlaySongs(songs, i, services.PlayFrom{ID: title}) },
			menus:  st.menus, art: ctx.Art, motion: ctx.Motion,
			current: currentSong(ctx), playing: ctx.Playing, restricted: ctx.Restricted,
			openArtist: d.pageOpener("/artist/"), openAlbum: d.pageOpener("/album/"),
		})
	})
}

// pageOpener opens the page of an item under prefix ("/artist/"), nil where
// that page is not ported.
func (d *discover) pageOpener(prefix string) func(id string) {
	if d.env.HasRoute == nil || !d.env.HasRoute(prefix+"x") {
		return nil
	}
	return func(id string) { d.env.Nav.Push(prefix + id) }
}

// chartsSkeleton is the charts overview while it loads: the top songs' heading
// and ranked rows, then a shelf of tiles, as flat bars at their real sizes.
func (d *discover) chartsSkeleton(c *ui.Context, m *measure, album bool) {
	ui.Column(c.Key("top-songs-skeleton")).FillWidth().Margin(8, 0, 40, 0).Role(ui.RoleNone).Children(func() {
		ui.Row(c).Height(32).Margin(0, 0, 12, 0).AlignItems(ui.Center).Children(func() { bar(c, 112, 16) })
		ui.Column(c.Key("rows")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
			for i := range 6 {
				rankPlaceholder(c, i, album)
			}
		})
	})
	d.shelvesSkeleton(c, m, false)
}

// chartPage is one chart in full, in pages of 50 as it scrolls.
func (d *discover) chartPage(c *ui.Context, r *ui.Route) {
	st := &d.chart
	key := r.Param("chart")
	genre, chartID := chartSearch(r)
	q := d.chartsFor(genre)
	pageKey := "chart/" + key + "|" + chartID + "|" + genre
	st.pg.body(c, pageKey, func(pane *kit.Pane, _ float32) {
		switch {
		case q.HasData:
			d.chartContent(c, r, pane, key, chartID, genre, q.Data)
		case q.Err != nil:
			d.pageError(c, i18n.T("Couldn't load this chart"), q.Err, q.Fetching, d.retry(keyCharts(genre)))
		default:
			d.masthead(c, "", "", true, nil)
			if key == services.ChartSongs {
				d.chartsSkeleton(c, &st.skel, st.pg.width >= rankAlbumAt)
			} else {
				d.shelvesSkeleton(c, &st.skel, false)
			}
		}
	})
}

func (d *discover) chartContent(c *ui.Context, r *ui.Route, pane *kit.Pane, key, chartID, genre string, page services.ChartsPage) {
	st := &d.chart
	genres := func() {
		d.genreChoice(c, page.Genres, genre, func(g string) { d.env.Nav.Replace(chartURL(key, g, chartID)) })
	}
	i := slices.IndexFunc(page.Charts, func(ch services.Chart) bool {
		return ch.Key == key && (chartID == "" || ch.ChartID == chartID)
	})
	if i < 0 {
		r.Title(i18n.T("Charts"))
		d.masthead(c, i18n.T("Charts"), "", false, genres)
		kit.EmptyState(c, i18n.T("No chart here"), i18n.T("Apple Music has no such chart for this genre."),
			kit.EmptyOpts{Action: &kit.EmptyAction{Label: i18n.T("All charts"), Run: func() { d.env.Nav.Push(pathCharts) }}})
		return
	}
	ch := page.Charts[i]
	title := chartTitle(ch)
	r.Title(title)
	sub := ""
	if genre != "" {
		if j := slices.IndexFunc(page.Genres, func(g services.Genre) bool { return g.ID == genre }); j >= 0 {
			sub = page.Genres[j].Name
		}
	}
	ctx := d.env.ItemsCtx(st.menus)
	listKey := key + "|" + ch.ChartID + "|" + genre
	st.list.pager.reset(listKey)
	pages := d.morePages(&st.list.pager, keyChart(key, ch.ChartID, genre), ch.Items, ch.Next, func(ctx context.Context, offset int) (services.ItemPage, error) {
		return d.api.ChartItems(ctx, key, ch.ChartID, genre, offset)
	})
	songs := key == services.ChartSongs
	var list []services.Song
	if songs {
		list = st.list.songList(listKey, &pages)
		d.masthead(c, title, sub, false, func() {
			d.playButtons(c, title, list)
			genres()
		})
	} else {
		d.masthead(c, title, sub, false, genres)
	}
	minTile := float32(160)
	if key == services.ChartMusicVideos {
		minTile = 240
	}
	d.itemList(c, &st.list, pane, &pages, list, ctx, listOpts{title: title, songs: songs, ranked: true, wide: st.pg.width >= rankAlbumAt, placeholders: 6, near: 12, min: minTile, menus: st.menus})
}
