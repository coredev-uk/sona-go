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
	skel   kit.WidthTracker
	shelf  map[string]*kit.CoverShelfState
	tracks kit.TrackListState
	songs  []services.Song
}

// chartPage is the state of one chart in full.
type chartPage struct {
	pg    pageView
	menus *kit.ActionMenus
	list  listState
	skel  kit.WidthTracker
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
		kit.PageHeader(c, i18n.T("Charts"), sub, actions)
		switch {
		case q.HasData:
			d.chartList(c, q.Data)
		case q.Err != nil:
			kit.QueryError(c, d.env.Queries, i18n.T("Couldn't load the charts"), q, keyCharts(genre))
		default:
			d.shelvesSkeleton(c, &st.skel, false)
		}
	})
}

// chartList is the overview's charts, in Apple's order: the top songs ranked,
// the others as shelves with "See all" to the chart in full.
func (d *discover) chartList(c *ui.Context, page services.ChartsPage) {
	st := &d.charts
	ctx := d.env.ItemsCtx(st.menus)
	for i, ch := range page.Charts {
		more := chartURL(ch.Key, page.Genre, ch.ChartID)
		if ch.Key == services.ChartSongs {
			d.topSongs(c, ch, more, ctx)
			continue
		}
		id := ch.Key + ":" + ch.ChartID
		sh := kit.Entry(&st.shelf, id)
		minTile := float32(160)
		if ch.Key == services.ChartMusicVideos {
			minTile = 240
		}
		kit.ItemShelf(c, sh, chartTitle(ch), ch.Items, ctx, kit.ItemShelfOpts{First: i == 0, More: more, Min: minTile})
	}
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
		ui.Row(c.Key("heading")).FillWidth().MinHeight(32).Margin(0, 0, 12, 0).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
			theme.StyleText(ui.Text(c, title), "heading").TextColor(theme.C("ink")).
				SingleLine().Role(ui.RoleHeading).Level(2)
			ui.Row(c.Key("play")).Gap(8).AlignItems(ui.Center).Children(func() { d.playButtons(c, title, songs) })
			ui.Box(c.Key("fill")).Grow(1)
			kit.ShelfSeeAll(c, "see-all", func() { d.env.Nav.Push(more) })
		})
		shown := songs[:min(len(songs), topSongs)]
		opts := d.env.TrackOpts(st.menus, ctx.Restricted)
		opts.Label = title
		opts.Columns = kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true}
		opts.OnPlay = func(i int) { d.env.Queue.PlaySongs(songs, i, services.PlayFrom{ID: title}) }
		kit.TrackList(c, &st.tracks, len(shown), func(i int) *services.Song { return &shown[i] }, opts)
	})
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
			kit.QueryError(c, d.env.Queries, i18n.T("Couldn't load this chart"), q, keyCharts(genre))
		default:
			d.shelvesSkeleton(c, &st.skel, false)
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
		kit.PageHeader(c, i18n.T("Charts"), "", genres)
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
		kit.PageHeader(c, title, sub, func() {
			d.playButtons(c, title, list)
			genres()
		})
	} else {
		kit.PageHeader(c, title, sub, genres)
	}
	minTile := float32(160)
	if key == services.ChartMusicVideos {
		minTile = 240
	}
	d.itemList(c, &st.list, pane, &pages, list, ctx, listOpts{title: title, songs: songs, placeholders: 6, near: 12, min: minTile, menus: st.menus})
}
