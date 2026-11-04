package library

import (
	"context"
	"net/url"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The grid's geometry, from the reference page (AppShell's main, PageHeader,
// CoverGrid, useCoverColumns) in DIPs.
const (
	padTop     = 28 // pt-7
	padBottom  = 48 // pb-12
	gapY       = 28
	textHeight = 8 + 20 + 18 // cover-to-title gap, the title's line and the artist's
	overscan   = 3           // rows built beyond the ones showing
	pageSize   = 200         // rows asked of the service at a time
	narrow     = 960         // the page's gutters shrink below this width
)

// sortOption is an entry of the Sort select.
type sortOption struct {
	sort  string
	label func() string
}

// sortOptions are the page's orders, in the select's order; the first is
// the default.
var sortOptions = []sortOption{
	{"dateAdded", func() string { return i18n.T("Recently added") }},
	{"title", func() string { return i18n.T("Title") }},
	{"artist", func() string { return i18n.T("Artist") }},
	{"releaseDate", func() string { return i18n.T("Release date") }},
}

// albumTypes are the Album type control's choices: "" is all.
var albumTypes = []string{"", "albums", "singles"}

// startsDesc: sorts that read best newest-first start descending.
func startsDesc(sort string) bool { return sort == "dateAdded" || sort == "releaseDate" }

// albumsQuery is what the URL says (?sort=…&desc=true&type=singles); an
// unknown value is dropped as v0.2.0's zod schema drops it.
type albumsQuery struct {
	sort string
	desc bool
	kind string // "", "albums" or "singles"
}

func queryOf(r *ui.Route) albumsQuery {
	q := albumsQuery{sort: sortOptions[0].sort}
	if s := r.Query("sort"); s != "" {
		for _, o := range sortOptions {
			if o.sort == s {
				q.sort = s
			}
		}
	}
	q.desc = startsDesc(q.sort)
	switch r.Query("desc") {
	case "true":
		q.desc = true
	case "false":
		q.desc = false
	}
	switch t := r.Query("type"); t {
	case "albums", "singles":
		q.kind = t
	}
	return q
}

// location is the page's URL for a query: the explicit sort and order, and
// the type when there is one.
func (q albumsQuery) location() string {
	v := url.Values{}
	v.Set("sort", q.sort)
	v.Set("desc", map[bool]string{true: "true", false: "false"}[q.desc])
	if q.kind != "" {
		v.Set("type", q.kind)
	}
	return albumsPath + "?" + v.Encode()
}

// albumsPage is the page's view state: the scroll place and the geometry
// of the last frame, which the next frame's virtualisation starts from.
type albumsPage struct {
	pos ui.ScrollState
	// The scroll container's size and the header's height as laid out in
	// the last frame; zero until there was one.
	width, height, headerH, rowH float32
	cols                         int
	// first and last are the rows showing.
	first, last int
	shown       albumsQuery
	typeIdx     int
	sortLabel   string
}

// albumsPage is LibraryAlbumsPage: the header (title, count, the type and
// sort controls), then the cover grid inside one scroll container that
// scrolls the header too. Its rows are built only while they show, and 3
// beyond (useContentVirtualizer's overscan): MyGo's List keeps its place by
// row, not by an offset the app can keep, so the grid is built by hand
// over Scroll and TrackScroll.
func (l *library) albumsPage(c *ui.Context, r *ui.Route) {
	l.toasts(c)
	q := queryOf(r)
	pg := &l.albums
	if pg.shown != q {
		if pg.shown != (albumsQuery{}) {
			pg.pos = ui.ScrollState{} // another list: from its top
		}
		pg.shown = q
	}
	first := l.page(q, 0)
	counts := store.Use(l.env.Queries, keyCounts, l.lib.Counts)
	total := first.Data.Total

	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if pg.width > 0 && pg.width < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	sc := ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		scrollbar := tk.Metric["scrollbar"]
		ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Children(func() {
			pg.headerH = l.header(c, q, first, total).Bounds().H
			switch {
			case first.Err != nil && !first.HasData:
				l.errorBlock(c, first.Err)
			case first.Loading || !first.HasData:
				l.grid(c, q, nil, 0, gutter)
			case total == 0:
				l.empty(c, q, counts.Data.Albums, gutter)
			default:
				l.grid(c, q, &first.Data, total, gutter)
			}
		})
	})
	b := sc.Bounds()
	if b.W != pg.width || b.H != pg.height {
		pg.width, pg.height = b.W, b.H
		c.Invalidate() // the grid's columns follow the width
	}
}

// page is one page of the albums of q: the key v0.2.0 used for it.
func (l *library) page(q albumsQuery, n int) store.Result[services.AlbumPage] {
	key := store.Key{"library", "albums", q.sort, q.desc, n}
	if q.kind != "" {
		key = append(key, q.kind)
	}
	return store.Use(l.env.Queries, key, func(ctx context.Context) (services.AlbumPage, error) {
		return l.lib.Albums(ctx, q.sort, q.desc, q.kind, n*pageSize, pageSize)
	}, store.Options{Stale: store.Forever})
}

// header is PageHeader: the title and its line about the data, with the
// controls at the right.
func (l *library) header(c *ui.Context, q albumsQuery, first store.Result[services.AlbumPage], total int) ui.Element {
	pg := &l.albums
	tk := theme.Current()
	return ui.Row(c).AlignItems(ui.End).Gap(24).Padding(0, 0, 24, 0).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			ui.Text(c, i18n.T("Albums")).FontSize(tk.Type["title"].Size).FixedLineHeight(tk.Type["title"].LineHeight).
				FontWeight(int(tk.Type["title"].Weight)).LetterSpacing(tk.Type["title"].Tracking).TextColor(theme.C("ink")).SingleLine()
			// A type with nothing in it still says so, so the header does not
			// jump when the filter changes.
			if sub := l.subline(q, first, total); sub != "" {
				ui.Text(c, sub).FontSize(tk.Type["secondary"].Size).FixedLineHeight(tk.Type["secondary"].LineHeight).
					TextColor(theme.C("ink-muted")).SingleLine().Margin(4, 0, 0, 0)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			// The Album type control, in the URL beside the sort.
			pg.typeIdx = 0
			for i, t := range albumTypes {
				if t == q.kind {
					pg.typeIdx = i
				}
			}
			idx := pg.typeIdx
			seg := ui.Segmented(c, &idx, i18n.T("All"), i18n.T("Albums"), i18n.T("Singles & EPs")).Label(i18n.T("Album type"))
			if seg.Changed() && idx != pg.typeIdx {
				next := q
				next.kind = albumTypes[idx]
				l.env.Nav.Replace(next.location())
			}

			labels := make([]string, len(sortOptions))
			for i, o := range sortOptions {
				labels[i] = o.label()
				if o.sort == q.sort {
					pg.sortLabel = labels[i]
				}
			}
			label := pg.sortLabel
			sel := ui.Select(c, &label, labels).Width(160).Label(i18n.T("Sort"))
			if sel.Changed() && label != pg.sortLabel {
				next := q
				for i, lab := range labels {
					if lab == label {
						next.sort = sortOptions[i].sort
					}
				}
				next.desc = startsDesc(next.sort)
				l.env.Nav.Replace(next.location())
			}
		})
	})
}

// subline is the line under the title: how many albums (or singles and
// EPs) the list has.
func (l *library) subline(q albumsQuery, first store.Result[services.AlbumPage], total int) string {
	if first.Loading || !first.HasData || (total == 0 && q.kind == "") {
		return ""
	}
	if q.kind == "singles" {
		return i18n.T("{0, plural, one {# single or EP} other {# singles & EPs}}", "0", total)
	}
	return i18n.T("{0, plural, one {# album} other {# albums}}", "0", total)
}

// grid is the cover grid. With no data yet it is the loading frame: flat
// tiles at the real sizes. Rows are built as they show.
func (l *library) grid(c *ui.Context, q albumsQuery, first *services.AlbumPage, total int, gutter float32) {
	pg := &l.albums
	tk := theme.Current()
	scrollbar := tk.Metric["scrollbar"]
	gapX, minTile := tk.Metric["tile-gap-x"], tk.Metric["tile-min"]
	// WebKitGTK's scroll bar took 12 DIPs of the page; MyGo's floats over it,
	// so the same gutter keeps the tiles the size they were.
	inner := pg.width - 2*gutter - scrollbar
	if pg.width <= 0 {
		return // the first frame measures; the next one builds the tiles
	}
	cols := max(1, int((inner+gapX)/(minTile+gapX)))
	tile := (inner - gapX*float32(cols-1)) / float32(cols)
	rowH := tile + textHeight + gapY

	count := total
	if first == nil {
		count = 12 // loading tiles
	}
	// Keep the same albums at the top when the columns change.
	top := float32(padTop) + pg.headerH
	if pg.cols != cols && pg.cols > 0 && pg.pos.Y > top {
		item := int((pg.pos.Y-top)/pg.rowH) * pg.cols
		pg.pos.Y = top + float32(item/cols)*rowH
	}
	pg.cols, pg.rowH = cols, rowH
	rows := (count + cols - 1) / cols
	y := pg.pos.Y - top
	pg.first = max(0, int(y/rowH))
	pg.last = min(rows-1, int((y+pg.height)/rowH))
	from, to := max(0, pg.first-overscan), min(rows-1, pg.last+overscan)

	// The pages the rows in view live in (and the next, as v0.2.0's paged
	// list asks for the neighbours of the ones showing).
	pages := map[int]*services.AlbumPage{}
	if first != nil {
		pages[0] = first
		for p := from * cols / pageSize; p <= min((to+1)*cols-1, count-1)/pageSize && p >= 0; p++ {
			if p == 0 {
				continue
			}
			if res := l.page(q, p); res.HasData {
				d := res.Data
				pages[p] = &d
			}
		}
	}

	ui.Box(c).Height(float32(from) * rowH)
	for r := from; r <= to; r++ {
		ui.Row(c.Key(r)).Height(rowH).Gap(gapX).AlignItems(ui.Start).Children(func() {
			for i := r * cols; i < min(count, (r+1)*cols); i++ {
				var a *services.Album
				if p := pages[i/pageSize]; p != nil && i%pageSize < len(p.Items) {
					a = &p.Items[i%pageSize]
				}
				l.tile(c, a, i, tile)
			}
		})
	}
	ui.Box(c).Height(float32(max(0, rows-1-to)) * rowH)
}

// tile is CoverTile: the cover with the play control over it, the title
// (underlined while the tile is hovered, with the explicit mark after it)
// and the artist. A tile whose page is still loading is a flat frame.
func (l *library) tile(c *ui.Context, a *services.Album, index int, size float32) {
	if a == nil {
		ui.Column(c.Key(index)).Width(size).Children(func() {
			ui.Box(c).Size(size, size).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
			ui.Box(c).Width(size*3/4).Height(14).Margin(8+3, 0, 0, 0).Radius(4).Background(theme.C("hover"))
			ui.Box(c).Width(size/2).Height(12).Margin(6+3, 0, 0, 0).Radius(4).Background(theme.C("hover"))
		})
		return
	}
	tk := theme.Current()
	col := ui.Column(c.Key(a.ID)).Width(size)
	hovered := col.Hovered()
	col.Children(func() {
		cover := ui.Box(c.Key("cover"))
		cover.Children(func() {
			kit.Artwork(c, l.env.Art, a.Art.Medium, size, kit.ArtOpts{Edge: true})
			// The play control, at the cover's bottom left, shown while the
			// tile is hovered or the control has the focus.
			play := kit.Button(c.Key("play"), "", kit.ButtonOpts{
				Variant:   kit.Play,
				Icon:      "play",
				A11yLabel: i18n.T("Play {playLabel}", "playLabel", a.Name),
			}).Absolute().Left(8).Bottom(8)
			if !hovered && !play.Focused() {
				play.Opacity(0)
			}
			if play.Clicked() {
				l.playAlbum(*a)
			}
		})
		ui.Row(c).Height(20).Margin(8, 0, 0, 0).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
			// No hover underline until the album page exists: it is the link affordance.
			ui.Text(c, a.Name).FontSize(tk.Type["label"].Size).FixedLineHeight(tk.Type["label"].LineHeight).
				FontWeight(int(tk.Type["label"].Weight)).TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
			if a.Explicit {
				explicitMark(c)
			}
		})
		ui.Text(c, a.Artist).FontSize(tk.Type["secondary"].Size).FixedLineHeight(tk.Type["secondary"].LineHeight).
			TextColor(theme.C("ink-muted")).SingleLine()
	})
}

// explicitMark is the small "E" beside explicit titles.
func explicitMark(c *ui.Context) {
	ui.Row(c).Size(14, 14).Radius(3).Center().Background(theme.C("ink-faint").Alpha(0.25)).
		Label(i18n.T("Explicit")).Tooltip(i18n.T("Explicit")).Role(ui.RoleImage).Children(func() {
		ui.Text(c, i18n.TC("explicit content mark", "E")).FontSize(9).FontWeight(600).TextColor(theme.C("ink-muted"))
	})
}
