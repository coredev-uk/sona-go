package library

import (
	"context"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The Artists page's geometry, from ArtistsPage.tsx in DIPs.
const (
	// artistsNarrowWidth: under this width of page the list and the detail
	// take turns instead of sitting side by side.
	artistsNarrowWidth = 880
	// artistsListWidth is the master list's width beside the detail.
	artistsListWidth = 320
	// artistsPageStep is how many artists a page-step key moves by.
	artistsPageStep = 10
	// artistsOverscan is how far beyond the viewport rows are built.
	artistsOverscan = 8 * artistsRowHeight
	// artistsGutter is the detail's side padding (px-8, px-6 under 960).
	artistsGutter       = 32
	artistsGutterNarrow = 24
)

// artistsRowHeight is artistRowHeight as a float for the layout sums.
const artistsRowHeight = float32(artistRowHeight)

var keyArtistIndex = store.Key{"library", "artists", "index"}

func keyArtistDetail(id string) store.Key { return store.Key{"library", "artist", id} }
func keyArtistAlbums(id string) store.Key { return store.Key{"library", "artist", id, "albums"} }

// artistsWant is the element the focus is asked to move to in the next
// frame: it is taken when that element is built.
type artistsWant int

const (
	artistsWantNone artistsWant = iota
	artistsWantFilter
	artistsWantList
	artistsWantBack
)

// artistsPage is the Artists page's view state.
type artistsPage struct {
	// gate is the scroll place of the gated page (header and gate).
	gate pageView

	query, cursor string

	// source is the data last indexed; index, byID (position of an ID in
	// index) follow it. gen counts the indexings.
	source []services.ArtistEntry
	index  []indexedArtist
	byID   map[string]int
	gen    int
	// The results of resQuery over index of generation resGen.
	resQuery string
	resGen   int
	matches  []indexedArtist
	rows     []artistRow
	tops     []float32
	position map[string]int // artist ID → place in matches
	rowOf    []int          // place in matches → row

	listPos, detailPos ui.ScrollState
	// listH is the height of the list's scroll box, width the page's, as
	// laid out in the last frame.
	listH, width float32
	// hl is the row the list last revealed; reveal asks for it.
	hl     artistsHighlight
	reveal bool
	// first and last are the rows built last frame.
	first, last int

	want artistsWant
	// focusDetail is an artist whose detail takes the focus once it shows.
	focusDetail string
	focusTries  int
	// inDetail: a narrow window shows the artist alone; seen: it was known
	// at the last frame.
	inDetail, seen bool
	// fromList: the open artist was opened from the list in this window, so
	// Back returns to it.
	fromList bool
	// announced is the filter result last read out.
	announced string

	// shown is the artist the detail's states belong to.
	shown                              string
	albums, singles                    kit.CoverState
	songs                              kit.TrackListState
	menus                              *kit.ActionMenus
	albumMenus, singleMenus, songMenus *kit.ActionMenus
}

// artistsHighlight is the row the list marks.
type artistsHighlight struct {
	id  string
	row int
}

// artistsView is the page as one frame builds it.
type artistsView struct {
	l  *library
	pg *artistsPage

	narrow bool
	// urlID is the URL's artist when the library holds it; openID the artist
	// the detail shows (beside the list, the first match stands in for none);
	// highlightID the row the list marks.
	urlID, openID, highlightID string
}

// reindex indexes data again when it is not the data last indexed.
func (pg *artistsPage) reindex(data []services.ArtistEntry) {
	if len(data) == len(pg.source) && (len(data) == 0 || &data[0] == &pg.source[0]) && pg.byID != nil {
		return
	}
	pg.source = data
	pg.index = indexArtists(data)
	pg.byID = make(map[string]int, len(pg.index))
	for i, a := range pg.index {
		pg.byID[a.entry.ID] = i
	}
	pg.gen++
}

// results works out the matches of the query over the index, their rows and
// the rows' places.
func (pg *artistsPage) results() {
	if pg.gen == pg.resGen && pg.query == pg.resQuery && pg.matches != nil {
		return
	}
	pg.resGen, pg.resQuery = pg.gen, pg.query
	pg.matches = filterArtists(pg.index, pg.query)
	if pg.matches == nil {
		pg.matches = []indexedArtist{}
	}
	pg.rows = listRows(pg.matches, strings.TrimSpace(pg.query) == "")
	pg.tops = rowTops(pg.rows)
	pg.position = make(map[string]int, len(pg.matches))
	pg.rowOf = make([]int, len(pg.matches))
	for i, r := range pg.rows {
		if r.artist != nil {
			pg.position[r.artist.entry.ID] = r.position
			pg.rowOf[r.position] = i
		}
	}
}

// artistsPage is LibraryArtistsPage: the header and the gate while the
// library has no artists to list, else the overview of master and detail.
func (l *library) artistsPage(c *ui.Context, r *ui.Route) {
	pg := &l.artists
	res := store.Use(l.env.Queries, keyArtistIndex, l.lib.ArtistIndex, store.Options{Stale: store.Forever})
	if res.HasData {
		pg.reindex(res.Data)
	}
	pending := !res.HasData && res.Err == nil
	var failure error
	if !res.HasData {
		failure = res.Err
	}
	if l.gated(gateOpts{total: len(pg.index), pending: pending, err: failure}) {
		l.scrollPage(c, &pg.gate, "artists", func(*kit.Pane) {
			kit.PageHeader(c, i18n.T("Artists"), "", nil)
			l.gate(c, gateOpts{
				total: len(pg.index), pending: pending, err: failure,
				retry:    func() { l.env.Queries.Invalidate(keyArtistIndex) },
				retrying: res.Fetching && res.Err != nil,
				kind:     kindArtists, frame: kit.FrameBox,
				empty: i18n.T("Every artist with a song or album in your library is listed here, A to Z."),
			})
		})
		return
	}
	l.artistsOverview(c, r, pending)
}

// artistsOverview is ArtistsOverview: the artists on the left, filtered as
// you type, the open artist's library albums on the right. The open artist
// is the URL's ?artist=, so Back, Forward and reload keep it; a window too
// narrow for both shows the list, then the artist with a way back.
func (l *library) artistsOverview(c *ui.Context, r *ui.Route, loading bool) {
	pg := &l.artists
	pg.results()
	v := &artistsView{l: l, pg: pg, narrow: pg.width > 0 && pg.width < artistsNarrowWidth}

	if id := r.Query("artist"); id != "" {
		if _, ok := pg.byID[id]; ok {
			v.urlID = id
		}
	}
	_, urlShown := pg.position[v.urlID]
	switch {
	case v.narrow:
		v.openID = v.urlID
	case v.urlID != "" && urlShown:
		v.openID = v.urlID
	case len(pg.matches) > 0:
		v.openID = pg.matches[0].entry.ID
	}
	cursorID := ""
	if _, ok := pg.position[pg.cursor]; ok && pg.cursor != "" {
		cursorID = pg.cursor
	} else if urlShown {
		cursorID = v.urlID
	}
	v.highlightID = cursorID
	if !v.narrow {
		v.highlightID = v.openID
	}

	// A narrow window changes view: focus follows to the detail's way back,
	// and returns to the filter.
	inDetail := v.narrow && v.urlID != ""
	if pg.seen && pg.inDetail != inDetail {
		pg.want = artistsWantFilter
		if inDetail {
			pg.want = artistsWantBack
		}
	}
	pg.inDetail, pg.seen = inDetail, true

	var detail *services.ArtistEntry
	if v.openID != "" && (!v.narrow || v.urlID != "") {
		detail = &pg.index[pg.byID[v.openID]].entry
	}

	root := ui.Row(c.Key("artists")).FillWidth().Grow(1).Basis(0).MinHeight(0).MinWidth(0).AlignItems(ui.Stretch)
	root.Children(func() {
		v.list(c, loading)
		if detail != nil {
			v.detail(c, *detail)
		}
	})
	if w := root.Bounds().W; w != pg.width {
		pg.width = w
		c.Invalidate() // the view follows the width
	}
}

// open puts the artist in the URL: a new history entry when the user chose
// it, the same one when a key walks the list.
func (v *artistsView) open(id string, replace bool) {
	v.pg.fromList = !replace
	loc := artistsPath + "?artist=" + url.QueryEscape(id)
	if replace {
		v.l.env.Nav.Replace(loc)
	} else {
		v.l.env.Nav.Push(loc)
	}
}

func (v *artistsView) pick(id string) {
	v.pg.cursor = id
	v.open(id, false)
	if !v.narrow { // keep the keys on the list after a click
		v.pg.want = artistsWantList
	}
}

// step moves the highlight by n artists (stopping at the ends).
func (v *artistsView) step(by int) {
	pg := v.pg
	n := len(pg.matches)
	if n == 0 {
		return
	}
	at, ok := pg.position[v.highlightID]
	to := min(max(at+by, 0), n-1)
	if v.highlightID == "" || !ok {
		to = 0
		if by < 0 {
			to = n - 1
		}
	}
	v.moveTo(pg.matches[to].entry.ID)
}

func (v *artistsView) ends(last bool) {
	n := len(v.pg.matches)
	if n == 0 {
		return
	}
	to := 0
	if last {
		to = n - 1
	}
	v.moveTo(v.pg.matches[to].entry.ID)
}

// moveTo: beside the detail the walk replaces the URL's artist, alone the
// list only moves its cursor.
func (v *artistsView) moveTo(id string) {
	if v.narrow {
		v.pg.cursor = id
		return
	}
	v.open(id, true)
}

// enter goes into the detail: opens the cursor's artist alone, or puts the
// open one in the URL and takes the focus to it beside the list.
func (v *artistsView) enter() {
	pg := v.pg
	id := v.openID
	if v.narrow {
		id = v.highlightID
		if id == "" && len(pg.matches) > 0 {
			id = pg.matches[0].entry.ID
		}
	}
	if id == "" {
		return
	}
	if v.narrow {
		v.open(id, false)
		return
	}
	if id != v.urlID {
		v.open(id, true)
	}
	pg.focusDetail, pg.focusTries = id, 0
}

func (v *artistsView) back() {
	if v.pg.fromList {
		v.l.env.Nav.Back()
		return
	}
	v.l.env.Nav.Replace(artistsPath)
}

// key is the keyboard the filter and the list share. atEnd reports the
// caret at the end of the filter's text (the field's → goes into the detail
// only from there); it is nil on the list.
func (v *artistsView) key(k ui.Key, mods ui.Modifiers, field bool, atEnd func() bool) bool {
	if mods&^ui.Shift != 0 {
		return false
	}
	switch k {
	case ui.KeyDown:
		v.step(1)
	case ui.KeyUp:
		v.step(-1)
	case ui.KeyPageDown:
		v.step(artistsPageStep)
	case ui.KeyPageUp:
		v.step(-artistsPageStep)
	case ui.KeyHome, ui.KeyEnd:
		if field { // the field keeps Home and End for its caret
			return false
		}
		v.ends(k == ui.KeyEnd)
	case ui.KeyEnter:
		v.enter()
	case ui.KeyRight:
		if field && !atEnd() {
			return false
		}
		v.enter()
	case ui.KeyEscape:
		if v.pg.query == "" {
			return false
		}
		v.pg.query, v.pg.want = "", artistsWantFilter
	default:
		return false
	}
	return true
}

// artistsText is a text of the theme's type scale.
func artistsText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// list is ArtistList: a title, the filter, then the artists A to Z under
// their letters, virtualised in its own scroll box.
func (v *artistsView) list(c *ui.Context, loading bool) {
	pg := v.pg
	tk := theme.Current()
	col := ui.Column(c.Key("list")).Hidden(pg.inDetail)
	if v.narrow {
		col.Grow(1).MinWidth(0)
	} else {
		col.Width(artistsListWidth).Shrink(0).BorderWidth(0, 1, 0, 0).BorderColor(theme.C("line"))
	}
	col.Children(func() {
		var in ui.Element
		ui.Column(c.Key("head")).Padding(28, 24, 12).Shrink(0).Children(func() {
			artistsText(c, i18n.T("Artists"), "page").TextColor(theme.C("ink")).SingleLine().Role(ui.RoleHeading).Level(1)
			ui.Row(c.Key("count")).MinHeight(18).Margin(4, 0, 0, 0).Children(func() {
				if line := v.countLine(); line != "" {
					artistsText(c, line, "sub").TextColor(theme.C("ink-muted")).FontFeatures("tnum")
				}
			})
			ui.Column(c.Key("filter")).Margin(16, 0, 0, 0).Children(func() {
				label := i18n.T("Filter artists")
				in = kit.SearchField(c, &pg.query, kit.SearchFieldOpts{Placeholder: label, A11yLabel: label})
			})
			in.Role(ui.RoleComboBox)
			// An element is only good for the frame that built it: the caret
			// of this frame is what a key finds.
			caretStart, caretEnd := in.TextSelection()
			in.OnTextKey(func(k ui.TextKey) bool {
				return v.key(k.Key, k.Mods, true, func() bool {
					n := utf8.RuneCountInString(k.Text())
					return caretStart == n && caretEnd == n
				})
			})
			// The arrows and page keys are the scroll boxes' too: they are
			// this field's as a shortcut.
			for _, mods := range [...]ui.Modifiers{0, ui.Shift} {
				for _, k := range [...]ui.Key{ui.KeyDown, ui.KeyUp, ui.KeyPageDown, ui.KeyPageUp} {
					if in.Shortcut(mods, k) {
						v.key(k, mods, true, nil)
					}
				}
			}
			if pg.want == artistsWantFilter {
				in.Focus()
				pg.want = artistsWantNone
			}
			v.announce(c)
		})

		var active ui.Element
		var hasActive bool
		switch {
		case loading:
			v.skeleton(c)
		case len(pg.matches) == 0:
			artistsText(c, i18n.T("No artist matches “{query}”.", "query", pg.query), "sub").
				TextColor(theme.C("ink-muted")).Padding(16, 24)
		default:
			active, hasActive = v.rowsView(c, tk)
		}
		// The list shows beside the filter while there are artists in it.
		in.Expanded(!loading && len(pg.matches) > 0)
		if hasActive {
			in.ActiveDescendant(active)
		}
	})
}

// countLine is the line under the title: how many artists the library
// holds, or how many of them match.
func (v *artistsView) countLine() string {
	pg := v.pg
	total := len(pg.index)
	if total == 0 {
		return ""
	}
	everyone := i18n.T("{total, plural, one {# artist} other {# artists}}", "total", total)
	if strings.TrimSpace(pg.query) == "" {
		return everyone
	}
	return i18n.T("{found} of {everyone}", "found", i18n.FormatNumber(float64(len(pg.matches))), "everyone", everyone)
}

// announce reads out the filter's result when it changes (a filter's result
// is not otherwise announced).
func (v *artistsView) announce(c *ui.Context) {
	pg := v.pg
	text := ""
	if pg.query != "" {
		if n := len(pg.matches); n > 0 {
			text = i18n.T("{matches, plural, one {# artist matches} other {# artists match}}", "matches", n)
		} else {
			text = i18n.T("No artist matches “{query}”", "query", pg.query)
		}
	}
	if text != "" && pg.announced != pg.query+"\x00"+text {
		c.Announce(text)
	}
	pg.announced = ""
	if text != "" {
		pg.announced = pg.query + "\x00" + text
	}
}

// skeleton is rows where an artist will be, while the library is read.
func (v *artistsView) skeleton(c *ui.Context) {
	ui.Column(c.Key("skeleton")).Padding(0, 12).Children(func() {
		for i := range 14 {
			ui.Row(c.Key(i)).Height(48).Padding(0, 12).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Box(c.Key("a")).Size(32, 32).Radius(16).Background(theme.C("hover")).Shrink(0)
				ui.Column(c.Key("t")).Gap(6).Children(func() {
					ui.Box(c.Key("n")).Size(128, 14).Radius(2).Background(theme.C("hover"))
					ui.Box(c.Key("l")).Size(80, 12).Radius(2).Background(theme.C("hover"))
				})
			})
		}
	})
}

// rowsView is the listbox: the rows in view and a few beyond, in a scroll
// box that scrolls by itself. It reports the highlighted row when built.
func (v *artistsView) rowsView(c *ui.Context, tk *theme.Tokens) (active ui.Element, hasActive bool) {
	pg := v.pg
	n := len(pg.rows)
	viewport := pg.listH
	if viewport <= 0 {
		viewport = 720
	}

	// Reveal the highlighted row (scrollToIndex align "auto").
	hl := artistsHighlight{id: v.highlightID, row: -1}
	if p, ok := pg.position[hl.id]; ok {
		hl.row = pg.rowOf[p]
	}
	if hl != pg.hl {
		pg.hl, pg.reveal = hl, hl.row >= 0
	}
	if pg.reveal && hl.row >= 0 {
		top, bottom := pg.tops[hl.row], pg.tops[hl.row+1]
		switch {
		case top < pg.listPos.Y:
			pg.listPos.Y = top
		case bottom > pg.listPos.Y+viewport:
			pg.listPos.Y = bottom - viewport
		}
		pg.reveal = false
	}

	y := pg.listPos.Y
	first := sort.Search(n, func(i int) bool { return pg.tops[i+1] > y-artistsOverscan })
	last := sort.Search(n, func(i int) bool { return pg.tops[i] >= y+viewport+artistsOverscan }) - 1
	first, last = min(first, n-1), min(last, n-1)
	pg.first, pg.last = first, last

	sc := ui.Scroll(c.Key("artist-list")).TrackScroll(&pg.listPos).Grow(1).Basis(0).MinHeight(0).Padding(0, 12, 24, 12).
		Role(ui.RoleListBox).Label(i18n.T("Artists in your library")).Focusable().
		Radius(tk.Radius["md"])
	focusRing := ui.Transparent
	if sc.FocusVisible() {
		focusRing = theme.C("ring")
	}
	sc.Border(2, focusRing)
	sc.TextCaret(ui.Rect{W: 1, H: 16}) // typing here takes text
	sc.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputKeyDown:
			return v.key(ev.Key, ev.Mods, false, nil)
		case ui.InputText:
			// A letter typed on the list goes to the filter.
			if ev.Text == "" || unicode.IsControl([]rune(ev.Text)[0]) {
				return false
			}
			pg.query += ev.Text
			pg.want = artistsWantFilter
			return true
		}
		return false
	})
	if pg.want == artistsWantList {
		sc.Focus()
		pg.want = artistsWantNone
	}
	sc.Children(func() {
		content := ui.Column(c.Key("rows")).FillWidth()
		content.Draw(func(_ *ui.Painter, _ ui.Rect) {
			// Rows near the edge of what is built come into view: build more.
			b := sc.Bounds()
			if b.H != pg.listH {
				pg.listH = b.H
				c.Invalidate()
				return
			}
			margin := 3 * artistsRowHeight
			y := pg.listPos.Y
			if (y < pg.tops[pg.first]+margin && pg.first > 0) || (y+b.H > pg.tops[pg.last+1]-margin && pg.last < len(pg.rows)-1) {
				c.Invalidate()
			}
		})
		content.Children(func() {
			if first > 0 {
				ui.Box(c.Key("above")).Height(pg.tops[first]).FillWidth().Shrink(0)
			}
			for i := first; i <= last; i++ {
				row := pg.rows[i]
				if row.artist == nil {
					v.letterRow(c, row.letter)
					continue
				}
				el := v.artistRow(c, row.artist.entry, row.position, row.artist.entry.ID == v.highlightID)
				if row.artist.entry.ID == v.highlightID {
					active, hasActive = el, true
				}
			}
			if rest := pg.tops[n] - pg.tops[last+1]; rest > 0 {
				ui.Box(c.Key("below")).Height(rest).FillWidth().Shrink(0)
			}
		})
	})
	if hasActive {
		sc.ActiveDescendant(active)
	}
	return active, hasActive
}

func (v *artistsView) letterRow(c *ui.Context, letter string) {
	ui.Row(c.Key("letter:"+letter)).Height(letterRowHeight).FillWidth().Padding(0, 12, 4).AlignItems(ui.End).Shrink(0).
		Role(ui.RoleNone).Children(func() {
		artistsText(c, letter, "meta").FontWeight(600).TextColor(theme.C("ink-muted")).Role(ui.RoleNone)
	})
}

// artistRow is ArtistOption: round portrait, name, and what the library
// holds of the artist.
func (v *artistsView) artistRow(c *ui.Context, a services.ArtistEntry, position int, selected bool) ui.Element {
	line := libraryLine(a.Albums, a.Songs)
	label := a.Name
	if line != "" {
		label += ", " + line
	}
	el := ui.Row(c.Key(a.ID)).Height(artistRowHeight).FillWidth().Padding(0, 12).Gap(12).AlignItems(ui.Center).Shrink(0).
		Radius(theme.Current().Radius["md"]).Role(ui.RoleOption).Label(label).Selected(selected).Cursor(ui.CursorPointer)
	switch {
	case selected:
		el.Background(theme.C("active"))
	case el.Hovered():
		el.Background(theme.C("hover"))
	}
	el.Children(func() {
		kit.PersonAvatar(c, v.l.env.Art, a.Name, a.Art, 32).Shrink(0)
		ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
			artistsText(c, a.Name, "ui").FontWeight(500).TextColor(theme.C("ink")).SingleLine().Role(ui.RoleNone)
			if line != "" {
				artistsText(c, line, "meta").TextColor(theme.C("ink-muted")).SingleLine().Role(ui.RoleNone)
			}
		})
	})
	if el.Clicked() {
		v.pick(a.ID)
	}
	return el
}

// detail is ArtistDetail: portrait, name, what the library holds, play and
// shuffle, the way to the full page, then the albums in the library as a
// grid of covers, newest release first, singles and EPs under their own
// heading. Only what Sona holds on this computer.
func (v *artistsView) detail(c *ui.Context, e services.ArtistEntry) {
	pg, l := v.pg, v.l
	if pg.shown != e.ID {
		pg.shown = e.ID
		pg.detailPos = ui.ScrollState{}
		pg.albums, pg.singles, pg.songs = kit.CoverState{}, kit.CoverState{}, kit.TrackListState{}
		pg.albumMenus, pg.singleMenus, pg.songMenus = nil, nil, nil
	}
	tk := theme.Current()
	gutter := float32(artistsGutter)
	if pg.width > 0 && pg.width < narrow {
		gutter = artistsGutterNarrow
	}
	albums := store.Use(l.env.Queries, keyArtistAlbums(e.ID), func(ctx context.Context) (services.ArtistAlbums, error) {
		return l.lib.ArtistAlbums(ctx, e.ID)
	}, store.Options{Stale: store.Forever})

	group := 3 // the group headings' level: h2 under an h1 artist
	nameLevel := 2
	if v.narrow {
		group, nameLevel = 2, 1
	}

	sc := ui.Scroll(c.Key("detail:" + e.ID)).TrackScroll(&pg.detailPos).Grow(1).Basis(0).MinWidth(0).MinHeight(0)
	sc.Children(func() {
		ui.Column(c).Padding(28, gutter+tk.Metric["scrollbar"], 48, gutter).Children(func() {
			if v.narrow {
				b := kit.Button(c.Key("back"), i18n.T("Artists"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Icon: "chevron-left"}).
					Margin(0, 0, 16, -12)
				if b.Clicked() {
					v.back()
				}
				if pg.want == artistsWantBack {
					b.Focus()
					pg.want = artistsWantNone
				}
			}
			ui.Row(c.Key("header")).FillWidth().Gap(24).AlignItems(ui.End).Children(func() {
				kit.PersonAvatar(c, l.env.Art, e.Name, e.Art, 96).Shrink(0)
				ui.Column(c.Key("who")).Grow(1).MinWidth(0).Padding(0, 0, 2).Children(func() {
					artistsText(c, e.Name, "display").TextColor(theme.C("ink")).MaxLines(2).Role(ui.RoleHeading).Level(nameLevel)
					line := i18n.T("Artist")
					if held := libraryLine(e.Albums, e.Songs); held != "" {
						line += " · " + held
					}
					artistsText(c, line, "meta").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
					ui.Row(c.Key("actions")).Wrap().Gap(8).AlignItems(ui.Center).Margin(16, 0, 0, 0).Children(func() {
						play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: e.Name, Shuffle: true, Disabled: e.Songs == 0})
						if play {
							v.play(e, false)
						}
						if shuffle {
							v.play(e, true)
						}
						if l.env.HasRoute != nil && l.env.HasRoute("/artist/x") {
							id := e.CatalogID
							if id == "" {
								id = e.ID
							}
							// It goes to another page: a link, as v0.2.0's was.
							page := kit.Button(c.Key("page"), i18n.T("Full artist page"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Role(ui.RoleLink)
							if page.Clicked() {
								l.env.Nav.Push("/artist/" + id)
							}
						}
						if menus := l.itemMenus(&pg.menus, nil); menus != nil && albums.HasData {
							menus.Trigger(c, kit.ActionTarget{Item: kit.ArtistItem(albums.Data.Artist), Name: e.Name}, kit.TriggerOpts{Header: true})
						}
					})
				})
			})
			ui.Column(c.Key("body")).FillWidth().Gap(40).Margin(32, 0, 0, 0).Children(func() {
				v.body(c, e, albums, group, sc)
			})
		})
	})
	if sc.Shortcut(0, ui.KeyEscape) {
		pg.want = artistsWantFilter
		if v.narrow {
			pg.want = artistsWantBack
		}
	}
	// The detail takes the focus: the first album once it is up.
	if pg.focusDetail == e.ID {
		nav := pg.albums.Nav()
		if pg.albums.Nav().Count == 0 {
			nav = pg.singles.Nav()
		}
		if pg.albums.Nav().Count == 0 && pg.singles.Nav().Count == 0 {
			nav = pg.songs.Nav()
		}
		if nav.Count > 0 {
			nav.Want(0)
			pg.focusDetail = ""
		} else if pg.focusTries++; pg.focusTries > 30 {
			pg.focusDetail = ""
		} else {
			c.Invalidate()
		}
	}
}

// from is what the artist's queue is played from.
func artistsFrom(id string) services.PlayFrom { return services.PlayFrom{Kind: "artist", ID: id} }

// play plays every song of the artist in the library, read only when asked.
func (v *artistsView) play(e services.ArtistEntry, shuffle bool) {
	l := v.l
	from := artistsFrom(e.ID)
	go func() {
		d, err := l.lib.Artist(context.Background(), e.ID)
		if err != nil {
			failToast(i18n.T("Couldn't load this artist"), err)
			return
		}
		l.env.Post(func() {
			if shuffle {
				l.env.Queue.ShuffleSongs(d.Songs, from)
			} else {
				l.env.Queue.PlaySongs(d.Songs, 0, from)
			}
		})
	}()
}

// body is what the detail lists: the albums, or the songs of an artist with
// no album, or why neither shows.
func (v *artistsView) body(c *ui.Context, e services.ArtistEntry, res store.Result[services.ArtistAlbums], group int, sc ui.Element) {
	pg, l := v.pg, v.l
	switch {
	case !res.HasData && res.Err != nil:
		kit.ErrorBlock(c, i18n.T("Couldn't read this artist's albums from this computer"), res.Err, kit.ErrorBlockOpts{
			OnRetry:  func() { l.env.Queries.Invalidate(keyArtistAlbums(e.ID)) },
			Retrying: res.Fetching,
		})
	case !res.HasData:
		v.tilesSkeleton(c)
	case len(res.Data.Albums) == 0:
		v.songsOnly(c, e, group, sc)
	default:
		var full, releases []services.Album
		for _, a := range res.Data.Albums {
			if a.Single { // Go decides what is a single or EP
				releases = append(releases, a)
			} else {
				full = append(full, a)
			}
		}
		if len(full) > 0 {
			v.section(c, "albums", i18n.T("Albums"), i18n.T("Albums by {name} in your library", "name", e.Name), full, &pg.albums, &pg.albumMenus, group)
		}
		if len(releases) > 0 {
			v.section(c, "singles", i18n.T("Singles and EPs"), i18n.T("Singles and EPs by {name} in your library", "name", e.Name), releases, &pg.singles, &pg.singleMenus, group)
		}
	}
}

// section is a heading over a grid of release tiles.
func (v *artistsView) section(c *ui.Context, key, title, label string, albums []services.Album, st *kit.CoverState, menus **kit.ActionMenus, group int) {
	l := v.l
	tile := l.albumTile()
	ui.Column(c.Key(key)).FillWidth().Role(ui.RoleGroup).Label(title).Children(func() {
		artistsText(c, title, "heading").TextColor(theme.C("ink")).Margin(0, 0, 16, 0).Role(ui.RoleHeading).Level(group)
		kit.CoverGrid(c, st, len(albums), func(i int) *kit.CoverItem {
			t := artistsReleaseTile(tile(&albums[i]), albums[i])
			return &t
		}, kit.CoverGridOpts{CoverOpts: kit.CoverOpts{
			Label: label, Art: l.env.Art, Motion: l.env.Motion, Restricted: l.restricted(),
			Menus: l.itemMenus(menus, &st.Sel),
		}})
	})
}

// tilesSkeleton is the grid where an artist's albums will be.
func (v *artistsView) tilesSkeleton(c *ui.Context) {
	l := v.l
	kit.CoverGrid(c, &v.pg.albums, 6, func(int) *kit.CoverItem { return nil }, kit.CoverGridOpts{
		CoverOpts: kit.CoverOpts{Label: i18n.T("Albums in your library"), Art: l.env.Art, Motion: l.env.Motion},
	})
}

// songsOnly is SongsOnly: an artist whose library holds songs but no album
// shows their songs where the albums would be.
func (v *artistsView) songsOnly(c *ui.Context, e services.ArtistEntry, group int, sc ui.Element) {
	pg, l := v.pg, v.l
	if e.Songs == 0 {
		artistsText(c, i18n.T("Add music by {name} to collect it here.", "name", e.Name), "ui").
			TextColor(theme.C("ink-muted")).MaxWidth(480)
		return
	}
	res := store.Use(l.env.Queries, keyArtistDetail(e.ID), func(ctx context.Context) (services.ArtistDetail, error) {
		return l.lib.Artist(ctx, e.ID)
	}, store.Options{Stale: store.Forever})
	switch {
	case !res.HasData && res.Err != nil:
		kit.ErrorBlock(c, i18n.T("Couldn't read this artist's songs from this computer"), res.Err, kit.ErrorBlockOpts{
			OnRetry:  func() { l.env.Queries.Invalidate(keyArtistDetail(e.ID)) },
			Retrying: res.Fetching,
		})
		return
	case !res.HasData:
		v.tilesSkeleton(c)
		return
	}
	songs := res.Data.Songs
	curID, curCatalog, playing := "", "", false
	if l.env.Play != nil {
		s := l.env.Play.State()
		if s.Current != nil {
			curID, curCatalog = s.Current.ID, s.Current.CatalogID
		}
		playing = s.State == store.StatePlaying
	}
	from := artistsFrom(e.ID)
	opts := kit.TrackListOpts{
		Label:      i18n.T("Songs by {name} in your library", "name", e.Name),
		Columns:    kit.TrackColumns{RowNumbers: true, Artwork: true, Album: true},
		OnPlay:     func(i int) { l.env.Queue.PlaySongs(songs, i, from) },
		Pane:       &kit.Pane{Pos: &pg.detailPos, Bounds: sc.Bounds},
		Current:    func(s *services.Song) bool { return kit.SongIsCurrent(s, curID, curCatalog) },
		Playing:    playing,
		Restricted: l.restricted(),
		Art:        l.env.Art,
		Motion:     l.env.Motion,
		Menus:      l.itemMenus(&pg.songMenus, &pg.songs.Sel),
	}
	if l.env.HasRoute != nil && l.env.HasRoute("/album/x") {
		opts.OpenAlbum = func(id string) { l.env.Nav.Push("/album/" + id) }
	}
	ui.Column(c.Key("songs")).FillWidth().Role(ui.RoleGroup).Label(i18n.T("Songs in your library")).Children(func() {
		artistsText(c, i18n.T("Songs in your library"), "heading").TextColor(theme.C("ink")).Margin(0, 0, 16, 0).
			Role(ui.RoleHeading).Level(group)
		kit.TrackList(c, &pg.songs, len(songs), func(i int) *services.Song { return &songs[i] }, opts)
	})
}

// releaseSuffix is Apple's " - Single" or " - EP" on a release's name.
var releaseSuffix = regexp.MustCompile(`^(.+?) - (Single|EP)$`)

// artistsReleaseTitle is a single's or EP's title without Apple's suffix,
// and which it was ("single", "ep", "" for neither): v0.2.0's releaseTitle
// (features/artist/artist.ts).
func artistsReleaseTitle(name string) (title, kind string) {
	m := releaseSuffix.FindStringSubmatch(name)
	if m == nil {
		return name, ""
	}
	if m[2] == "EP" {
		return m[1], "ep"
	}
	return m[1], "single"
}

// artistsYear is the year of a release date ("YYYY-MM-DD", a year alone or "").
func artistsYear(date string) string {
	if len(date) < 4 {
		return ""
	}
	for _, r := range date[:4] {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return date[:4]
}

// artistsReleaseTile is an album's tile in the artist's detail (v0.2.0's
// releaseTile): the title without a single's or EP's suffix, the year and
// the kind under it.
func artistsReleaseTile(t kit.CoverItem, a services.Album) kit.CoverItem {
	title, kind := artistsReleaseTitle(a.Name)
	year := artistsYear(a.ReleaseDate)
	t.Subtitle = year
	// As v0.2.0's albumTile: the album's motion artwork, and the release date
	// of an album not out yet.
	t.Motion = a.Motion
	t.Badge = kit.ReleaseBadge(a, time.Now())
	label := ""
	switch {
	case kind == "ep":
		label = i18n.T("EP")
	case kind == "single" || a.Single:
		label = i18n.T("Single")
	}
	if label == "" {
		return t
	}
	t.Title = title
	if year != "" {
		t.Subtitle = year + " · " + label
	} else {
		t.Subtitle = label
	}
	return t
}
