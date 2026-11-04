package kit

import (
	"hash/maphash"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of a track table (v0.2.0's TrackList), in DIPs.
const (
	// TrackRowHeight is a row's height (docs/design.md).
	TrackRowHeight = 44
	// trackHeaderHeight is the header row's.
	trackHeaderHeight = 32
	trackPadX         = 8
	trackGap          = 16
	trackNumberW      = 40
	trackDurationW    = 56
	trackActionsW     = 32
	// trackOverscan is how many rows beyond the ones showing are built.
	trackOverscan = 12
)

// TrackSort keys of the sortable columns.
const (
	SortTitle  = "title"
	SortArtist = "artist"
	SortAlbum  = "album"
)

// TrackColumns says which columns a list shows.
type TrackColumns struct {
	// RowNumbers numbers the rows 1…n (a playlist); else the album's track
	// numbers, falling back to the position.
	RowNumbers bool
	Artwork    bool
	Artist     bool
	Album      bool
}

// TrackSortState is the list's order, for the sortable headers (Title,
// Artist, Album): Key is the active column ("" none) and OnSort is called
// with the key of a clicked header.
type TrackSortState struct {
	Key    string
	Desc   bool
	OnSort func(key string)
}

// SongIsCurrent is v0.2.0's isCurrent: the queue's current item is the song
// by ID, or by catalogue ID.
func SongIsCurrent(s *services.Song, currentID, currentCatalogID string) bool {
	if s == nil || (currentID == "" && currentCatalogID == "") {
		return false
	}
	return currentID == s.ID || (s.CatalogID != "" && currentCatalogID == s.CatalogID)
}

// TrackListOpts are the options of TrackList.
type TrackListOpts struct {
	// Label names the table: "Songs in OK Computer".
	Label   string
	Columns TrackColumns
	// Hold keeps a press outside the list from clearing the selection (a
	// feature's own dialog); open menus and a pending confirmation always do.
	Hold func() bool
	// OnPlay plays from row index with the whole list as the queue.
	OnPlay func(index int)
	// Sort makes Title, Artist and Album headers sortable.
	Sort *TrackSortState
	// Discs groups album tracks under "Disc n" when there is more than one.
	Discs bool
	// Highlight is a song (ID or catalogue ID) to mark and scroll to
	// (from a shared link).
	Highlight string
	// Popular holds catalogue IDs marked "Popular" after the title.
	Popular map[string]bool
	// OnRange reports the rows built (the whole list when not virtual).
	OnRange func(start, end int)
	// Container is the collection the songs belong to, for row actions.
	Container *ActionContainer
	// AddedBy makes the "Added by" column of a collaborative playlist: who
	// added row i (nil when Apple doesn't say).
	AddedBy func(i int) *services.SocialProfile
	// Subline is the line under a row's title, in place of the default (the
	// artist, for a list numbered by row without an artist column); "" keeps
	// the row on one line.
	Subline func(s *services.Song) string
	// Title is the title shown in place of the song's name (a classical
	// movement under its work's heading); the name stays the row's label.
	Title func(s *services.Song) string
	// Heading is a heading before a row, below any disc heading: a
	// classical album's works. Virtual lists show none.
	Heading func(s *services.Song, prev *services.Song) string
	// Current says whether a song is the one playing (SongIsCurrent), and
	// Playing whether it plays (else it is paused).
	Current func(s *services.Song) bool
	Playing bool
	// Restricted: the account may not play explicit songs.
	Restricted bool

	Art    *ArtLoader
	Menus  *ActionMenus
	Motion *MotionPolicy
	// NoPopup: the page draws Menus.Popup itself.
	NoPopup bool
	// Pane is the scroll container the list is built in; a list of more
	// than VirtualFrom rows in a pane is built by row, as it shows.
	Pane *Pane

	// OpenArtist, OpenAlbum and OpenProfile open the page of a row's artist,
	// album or "added by" profile (the cells are links).
	OpenArtist  func(id string)
	OpenAlbum   func(id string)
	OpenProfile func(id string)
}

// TrackListState is the state of one track list: selection, keyboard
// navigation, and what the last frame built.
type TrackListState struct {
	// Sel is the selection of the rows (selection keys are song IDs so
	// they survive re-sorting; a repeated song gets "#n").
	Sel Selection
	nav GridNav

	rows   trackRows
	seed   maphash.Seed
	hash   uint64
	hashed bool

	top float32 // the body's offset in its pane's content
	// workH is the laid-out height of each work heading, by the row it
	// precedes, as of the last frame.
	workH map[int]float32
	// drawTop is that offset as the last paint saw it (one frame's position
	// and scroll offset together); drawn: there was a paint.
	drawTop  float32
	drawn    bool
	reported [2]int
	// The highlighted song: its row once found, and whether the list has
	// been scrolled to it.
	hlFor      string
	hlIndex    int
	hlScrolled bool
}

// Nav is the keyboard navigation.
func (st *TrackListState) Nav() *GridNav { return &st.nav }

// trackRows is Rows over the list's songs: a song's key is its ID, "#n" for
// its nth copy.
type trackRows struct {
	keys  []string
	songs []*services.Song
}

func (r *trackRows) Len() int { return len(r.keys) }
func (r *trackRows) Key(i int) string {
	if i < 0 || i >= len(r.keys) {
		return ""
	}
	return r.keys[i]
}

func (r *trackRows) item(i int) (Item, bool) {
	if i < 0 || i >= len(r.songs) || r.songs[i] == nil {
		return Item{}, false
	}
	return SongItem(*r.songs[i]), true
}

// sync brings the key list up to date with the songs as at gives them: the
// keys are rebuilt only when a pass over the song IDs says they changed.
func (st *TrackListState) sync(count int, at func(i int) *services.Song) {
	if st.seed == (maphash.Seed{}) {
		st.seed = maphash.MakeSeed()
	}
	var h maphash.Hash
	h.SetSeed(st.seed)
	for i := range count {
		if s := at(i); s != nil {
			h.WriteString(s.ID)
		}
		h.WriteByte(0)
	}
	sum := h.Sum64()
	if st.hashed && sum == st.hash && len(st.rows.keys) == count {
		return
	}
	st.hash, st.hashed = sum, true
	st.rows.keys = make([]string, count)
	st.rows.songs = make([]*services.Song, count)
	seen := map[string]int{}
	for i := range count {
		s := at(i)
		st.rows.songs[i] = s
		if s == nil || s.ID == "" {
			continue
		}
		seen[s.ID]++
		key := s.ID
		if n := seen[s.ID]; n > 1 {
			key += "#" + strconv.Itoa(n)
		}
		st.rows.keys[i] = key
	}
	// The songs themselves may be replaced by newer copies of the same
	// songs (a page that loaded); the items follow at.
}

// TrackList is a track table (v0.2.0's TrackList): a header row and fixed 44
// DIP rows with no dividers, the playing track marked in place. Double-click
// or Enter plays from that row with the list as the queue. Click, Ctrl+click
// and Shift+click (or Shift+arrows, Ctrl+A) select rows; right-click or
// Shift+F10 opens the row's menu, which acts on the whole selection when the
// row is in it; rows drag onto playlists and the queue. The list is one Tab
// stop: the focused row, with the arrows, Page Up and Down, Home and End
// moving it.
//
// at returns song i, nil for a row whose page is still loading. In a pane
// with more than VirtualFrom rows only the rows in view and twelve beyond are
// built, and no disc or work headings are shown.
func TrackList(c *ui.Context, st *TrackListState, count int, at func(i int) *services.Song, o TrackListOpts) {
	tk := theme.Current()
	st.sync(count, at)
	st.Sel.Hold = collectionsHold(o.Menus, o.Hold)
	st.Sel.Rows(&st.rows, st.rows.item)
	virtual := count > VirtualFrom && o.Pane != nil
	st.nav.Count, st.nav.Columns = count, 1
	st.nav.Page = max(1, int(windowHeight(c)/TrackRowHeight)-2)
	st.nav.Ends = true
	st.nav.ScrollTo = nil

	root := ui.Column(c.Key("kit.tracks")).FillWidth().Role(ui.RoleTable).Label(o.Label)
	if o.Pane != nil {
		root.Draw(func(_ *ui.Painter, r ui.Rect) {
			// Paint runs after this frame's layout: the box and the scroll
			// offset belong to the same frame.
			top := r.Y - o.Pane.Bounds().Y + o.Pane.Pos.Y + trackHeaderHeight + 4
			if !st.drawn || top != st.drawTop {
				st.drawTop, st.drawn = top, true
				c.Invalidate()
			}
		})
	}
	root.Children(func() {
		st.header(c, &o, tk)
		body := ui.Column(c.Key("body")).Padding(4, 0, 0, 0)
		body.Children(func() {
			var built []int
			if o.Pane != nil {
				if st.drawn {
					st.top = st.drawTop
				} else {
					st.top = root.Bounds().Y - o.Pane.Bounds().Y + o.Pane.Pos.Y + trackHeaderHeight + 4
				}
			}
			if virtual {
				built = st.window(o.Pane, count)
				st.nav.ScrollTo = func(i int) { st.scrollTo(o.Pane, i, false, 0) }
			} else {
				built = make([]int, count)
				for i := range built {
					built[i] = i
				}
			}
			winFirst, winLast := 0, count-1
			if len(built) > 0 {
				winFirst, winLast = built[0], built[len(built)-1] // not the dragged row's
			}
			if i, ok := DragPinned(st); ok && i < count && !containsInt(built, i) {
				built = append(built, i)
				slices.Sort(built)
			}
			st.highlight(c, &o, count, at, st.drawn || root.Bounds().H > 0)
			multiDisc := o.Discs && !virtual && st.multiDisc(count, at)
			prev := -1
			for _, i := range built {
				if gap := i - prev - 1; gap > 0 {
					ui.Box(c.Key(gapKey(prev))).Height(float32(gap) * TrackRowHeight)
				}
				prev = i
				s := at(i)
				if !virtual {
					st.headings(c, &o, i, s, at, multiDisc)
				}
				st.row(c, &o, i, s, tk)
			}
			if gap := count - 1 - prev; virtual && gap > 0 {
				ui.Box(c.Key(gapKey(count))).Height(float32(gap) * TrackRowHeight)
			}
			st.report(virtual, winFirst, winLast, count, o.OnRange)
		})
		st.nav.Tick(c)
		st.Sel.OutsidePress(root)
		if o.Menus != nil && !o.NoPopup {
			o.Menus.Popup(c)
		}
	})
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func windowHeight(c *ui.Context) float32 {
	_, h := c.Size()
	return h
}

// window works out the rows of a virtual list to build: those in view and
// trackOverscan beyond.
func (st *TrackListState) window(p *Pane, count int) []int {
	pane := p.Bounds()
	y := p.Pos.Y - st.top
	h := pane.H
	if h <= 0 {
		h = TrackRowHeight * 20
	}
	first := max(0, int(y/TrackRowHeight))
	last := min(count-1, int((y+h)/TrackRowHeight))
	from, to := max(0, first-trackOverscan), min(count-1, last+trackOverscan)
	out := make([]int, 0, max(0, to-from+1))
	for i := from; i <= to; i++ {
		out = append(out, i)
	}
	return out
}

// scrollTo scrolls the pane as little as shows row i, or centres it; above is
// the height of the disc and work headings over the row.
func (st *TrackListState) scrollTo(p *Pane, i int, center bool, above float32) {
	pane := p.Bounds()
	top := st.top + float32(i)*TrackRowHeight + above
	bottom := top + TrackRowHeight
	switch {
	case center && pane.H > 0:
		p.Pos.Y = max(0, top+TrackRowHeight/2-pane.H/2)
	case top < p.Pos.Y:
		p.Pos.Y = max(0, top)
	case pane.H > 0 && bottom > p.Pos.Y+pane.H:
		p.Pos.Y = bottom - pane.H
	}
}

// highlight finds the highlighted song once it has loaded, gives it the
// initial focus and scrolls it to the centre.
func (st *TrackListState) highlight(c *ui.Context, o *TrackListOpts, count int, at func(i int) *services.Song, laidOut bool) {
	if o.Highlight == "" {
		st.hlFor, st.hlIndex, st.hlScrolled = "", -1, false
		return
	}
	if st.hlFor != o.Highlight {
		st.hlFor, st.hlIndex, st.hlScrolled = o.Highlight, -1, false
	}
	if st.hlIndex < 0 {
		for i := range count {
			if s := at(i); s != nil && (s.ID == o.Highlight || s.CatalogID == o.Highlight) {
				st.hlIndex = i
				st.nav.SetFocus(i)
				break
			}
		}
	}
	if st.hlIndex >= 0 && !st.hlScrolled {
		if o.Pane != nil && !laidOut {
			c.Invalidate() // the list is not laid out yet: its offset is unknown
			return
		}
		st.hlScrolled = true
		if o.Pane != nil {
			var above float32 // headings are built only in a list that is not virtual
			if o.Discs && !(count > VirtualFrom) {
				multiDisc := st.multiDisc(count, at)
				for j := 0; j <= st.hlIndex; j++ {
					above += st.headingHeight(o, j, at, multiDisc)
				}
			}
			st.scrollTo(o.Pane, st.hlIndex, true, above)
		}
		c.Invalidate()
	}
}

func (st *TrackListState) multiDisc(count int, at func(i int) *services.Song) bool {
	if count == 0 {
		return false
	}
	first := at(0)
	if first == nil {
		return false
	}
	for i := 1; i < count; i++ {
		if s := at(i); s != nil && s.DiscNumber != first.DiscNumber {
			return true
		}
	}
	return false
}

// report tells OnRange which rows are built, when that changed.
func (st *TrackListState) report(virtual bool, winFirst, winLast, count int, fn func(start, end int)) {
	if fn == nil || count == 0 {
		return
	}
	first, last := 0, count-1
	if virtual {
		first, last = winFirst, winLast
	}
	if st.reported == [2]int{first + 1, last + 1} {
		return
	}
	st.reported = [2]int{first + 1, last + 1}
	fn(first, last)
}

// headings builds the "Disc n" heading and the work heading before row i.
func (st *TrackListState) headings(c *ui.Context, o *TrackListOpts, i int, s *services.Song, at func(i int) *services.Song, multiDisc bool) {
	if s == nil {
		return
	}
	tk := theme.Current()
	var prev *services.Song
	if i > 0 {
		prev = at(i - 1)
	}
	disc := multiDisc && (i == 0 || prev == nil || prev.DiscNumber != s.DiscNumber)
	if disc {
		top := float32(20)
		if i == 0 {
			top = 8
		}
		ui.Text(c.Key("disc-"+strconv.Itoa(i)), i18n.T("Disc {disc}", "disc", s.DiscNumber)).
			FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).FontWeight(600).
			TextColor(theme.C("ink-muted")).Padding(top, trackPadX, 8).Role(ui.RoleHeading).Level(3)
	}
	if o.Heading == nil {
		return
	}
	if h := o.Heading(s, prev); h != "" {
		top, level := float32(20), 3
		if i == 0 || disc {
			top = 8
		}
		if multiDisc {
			level = 4
		}
		e := ui.Text(c.Key("work-"+strconv.Itoa(i)), h).
			FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).FontWeight(500).
			TextColor(theme.C("ink")).Padding(top, trackPadX, 6).Role(ui.RoleHeading).Level(level)
		// A work heading may wrap: its laid-out height (last frame's) is what
		// BlockTop sums.
		if b := e.Bounds(); b.H > 0 {
			if st.workH == nil {
				st.workH = map[int]float32{}
			}
			st.workH[i] = b.H
		}
	}
}

// columns lays a row's or the header's cells out: the widths of
// gridTemplate (# 40, title 2fr, artist 1fr, album 1fr, added by 0.8fr,
// duration 56, actions 32), 16 apart.
func (o *TrackListOpts) cell(e ui.Element, kind int) ui.Element {
	switch kind {
	case cellNumber:
		return e.Width(trackNumberW).Shrink(0)
	case cellTitle:
		return e.Grow(2).Basis(0).MinWidth(0)
	case cellArtist, cellAlbum:
		return e.Grow(1).Basis(0).MinWidth(0)
	case cellAddedBy:
		return e.Grow(0.8).Basis(0).MinWidth(0)
	case cellDuration:
		return e.Width(trackDurationW).Shrink(0)
	}
	return e.Width(trackActionsW).Shrink(0) // cellActions
}

const (
	cellNumber = iota
	cellTitle
	cellArtist
	cellAlbum
	cellAddedBy
	cellDuration
	cellActions
)

// header builds the column headers: Title, Artist and Album are sort buttons
// when the list sorts.
func (st *TrackListState) header(c *ui.Context, o *TrackListOpts, tk *theme.Tokens) {
	meta := tk.Type["meta"]
	row := ui.Row(c.Key("header")).Height(trackHeaderHeight).PaddingX(trackPadX).Gap(trackGap).AlignItems(ui.Center).
		Role(ui.RoleRow)
	row.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y + r.H - 1, W: r.W, H: 1}, theme.C("line"), 0)
	})
	label := func(e ui.Element) ui.Element {
		return e.FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-faint")).SingleLine()
	}
	row.Children(func() {
		o.cell(ui.Row(c.Key("h-num")).Role(ui.RoleColumnHeader).Justify(ui.End), cellNumber).Children(func() { label(ui.Text(c, "#")) })
		st.headerCell(c, o, cellTitle, i18n.T("Title"), SortTitle, meta)
		if o.Columns.Artist {
			st.headerCell(c, o, cellArtist, i18n.T("Artist"), SortArtist, meta)
		}
		if o.Columns.Album {
			st.headerCell(c, o, cellAlbum, i18n.T("Album"), SortAlbum, meta)
		}
		if o.AddedBy != nil {
			st.headerCell(c, o, cellAddedBy, i18n.T("Added by"), "", meta)
		}
		o.cell(ui.Row(c.Key("h-dur")).Role(ui.RoleColumnHeader).Justify(ui.End), cellDuration).Children(func() {
			ui.Icon(c, theme.Icon("clock-3")).FontSize(14).TextColor(theme.C("ink-faint")).Label(i18n.T("Duration"))
		})
		o.cell(ui.Box(c.Key("h-act")).Role(ui.RoleColumnHeader).Label(i18n.T("Actions")), cellActions).Height(1)
	})
}

// headerCell is one column header: a sort button when the list sorts by key.
func (st *TrackListState) headerCell(c *ui.Context, o *TrackListOpts, kind int, text, key string, meta theme.TypeStyle) {
	cell := o.cell(ui.Row(c.Key("h-"+text)).AlignItems(ui.Center), kind)
	if key == "" || o.Sort == nil {
		cell.Role(ui.RoleColumnHeader)
	} else {
		// A column header is a leaf on the accessibility tree, so the sort
		// button inside it would not be on it: the button stands for the
		// column, named by its title with its direction as the value.
		cell.Role(ui.RoleNone)
	}
	cell.Children(func() {
		if key == "" || o.Sort == nil {
			ui.Text(c, text).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).TextColor(theme.C("ink-faint")).SingleLine()
			return
		}
		active := o.Sort.Key == key
		btn := ui.ButtonBase(c.Key("sort-" + key)).Height(24).Gap(4).MarginX(-4).PaddingX(4).Radius(theme.Current().Radius["sm"])
		state := "none"
		if active {
			state = "ascending"
			if o.Sort.Desc {
				state = "descending"
			}
		}
		fg := theme.C("ink-faint")
		if active || btn.Hovered() {
			fg = theme.C("ink")
		}
		btn.Label(text).Value(state).TextColor(fg)
		btn.Children(func() {
			ui.Text(c, text).FontSize(meta.Size).FixedLineHeight(meta.LineHeight).SingleLine()
			if active {
				name := "arrow-up"
				if o.Sort.Desc {
					name = "arrow-down"
				}
				ui.Icon(c, theme.Icon(name)).FontSize(12)
			}
		})
		if btn.Clicked() && o.Sort.OnSort != nil {
			o.Sort.OnSort(key)
		}
	})
}

// fillRun paints a selected row's tint: rounded alone, and a run of adjacent
// rows as one block with only its outer corners rounded (one path, so the
// translucent tint never doubles where rows meet).
func fillRun(p *ui.Painter, r ui.Rect, run Run, col ui.Color, radius float32) {
	if run == RunNone {
		return
	}
	top, bottom := radius, radius
	switch run {
	case RunFirst:
		bottom = 0
	case RunLast:
		top = 0
	case RunMiddle:
		top, bottom = 0, 0
	}
	if top == 0 && bottom == 0 {
		p.Fill(r, col, 0)
		return
	}
	const k = 0.5523 // a quarter circle as a cubic curve
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
	var path ui.Path
	path.MoveTo(x0+top, y0).LineTo(x1-top, y0)
	path.CubeTo(x1-top+top*k, y0, x1, y0+top-top*k, x1, y0+top)
	path.LineTo(x1, y1-bottom)
	path.CubeTo(x1, y1-bottom+bottom*k, x1-bottom+bottom*k, y1, x1-bottom, y1)
	path.LineTo(x0+bottom, y1)
	path.CubeTo(x0+bottom-bottom*k, y1, x0, y1-bottom+bottom*k, x0, y1-bottom)
	path.LineTo(x0, y0+top)
	path.CubeTo(x0, y0+top-top*k, x0+top-top*k, y0, x0+top, y0).Close()
	p.FillPath(&path, col)
}

// row builds row i: a skeleton while its page loads.
func (st *TrackListState) row(c *ui.Context, o *TrackListOpts, i int, s *services.Song, tk *theme.Tokens) {
	hover := theme.C("hover")
	if s == nil {
		st.skeletonRow(c, o, i, hover)
		return
	}
	cols := o.Columns
	withheld := o.Restricted && s.Explicit
	disabled := !s.Playable || withheld
	current := o.Current != nil && o.Current(s)
	run := st.Sel.Run(i)
	highlighted := i == st.hlIndex && o.Highlight != ""
	key := st.rows.Key(i)
	number := i + 1
	if !cols.RowNumbers && s.TrackNumber > 0 {
		number = s.TrackNumber
	}
	sub := ""
	switch {
	case o.Subline != nil:
		sub = o.Subline(s)
	case !cols.Artist && cols.RowNumbers:
		sub = s.Artist
	}
	title := s.Name
	if o.Title != nil {
		title = o.Title(s)
	}
	target := ActionTarget{Item: SongItem(*s), Name: s.Name, Key: key}
	if o.Container != nil {
		cont := *o.Container
		cont.Index = i
		target.Container = &cont
	}
	ink, muted, faint := theme.C("ink"), theme.C("ink-muted"), theme.C("ink-faint")
	if disabled {
		ink, muted = faint, faint
	}

	_, src := DragItem(c, DragOpts{
		Key:    s.ID + "@" + strconv.Itoa(i),
		Scope:  st,
		Index:  i,
		Art:    o.Art,
		Row:    true,
		Button: false,
		Payload: func() *DragPayload {
			if key == "" || withheld {
				return nil
			}
			items, _, group := st.Sel.GroupWith(key)
			label := s.Name
			if group {
				label = DescribeItems(items)
			} else {
				items = []Item{SongItem(*s)}
			}
			return &DragPayload{Items: items, Label: label, Art: s.Art.Small}
		},
		Outer: func(w ui.Element) {
			w.Height(TrackRowHeight).Shrink(0).Role(ui.RoleNone)
			if withheld {
				w.Tooltip(i18n.T("Explicit content is restricted on this account"))
			}
			bg := hover.Alpha(0)
			switch {
			case highlighted:
				bg = theme.C("active")
			case w.Hovered() && run == RunNone:
				bg = hover
			}
			if !highlighted && !theme.ReducedMotion() {
				m := tk.Motion["feedback"]
				bg = w.AnimateColor("kit.bg", bg, ui.Animation{Duration: m.Duration, Ease: m.Ease})
			}
			radius := tk.Radius["md"]
			sel := theme.C("ink").Alpha(0.12)
			w.Draw(func(p *ui.Painter, r ui.Rect) {
				if bg.A > 0 {
					p.Fill(r, bg, radius)
				}
				fillRun(p, r, run, sel, radius)
			})
		},
		Source: func(e ui.Element) {
			e.PaddingX(trackPadX).Gap(trackGap).Radius(tk.Radius["md"]).Role(ui.RoleRow).Label(s.Name).Checked(run != RunNone)
			if i == st.nav.Focus() {
				e.Focusable()
			}
		},
	}, func(w, src ui.Element) {
		// A Cmd/Shift+click on a link selects the row, as anywhere on it.
		selectRow := func(m ClickMods) {
			st.nav.SetFocus(i)
			st.nav.Want(i)
			st.Sel.Click(i, m)
		}
		hot := w.Hovered() || src.FocusWithin()
		o.cell(ui.Row(c.Key("num")).Role(ui.RoleCell).Justify(ui.End).AlignItems(ui.Center), cellNumber).Children(func() {
			if current {
				EqualiserBars(c, o.Playing, o.Motion)
				return
			}
			// The play button is built at rest too, over the number and
			// invisible, so that assistive technology finds and presses it
			// (the pointer and Tab ignore it until the row is hovered).
			shown := w.Hovered() && !disabled
			if !disabled {
				play := ui.ButtonBase(c.Key("play")).Width(24).FillHeight().Label(i18n.T("Play {name}", "name", s.Name))
				if !shown {
					play.Absolute().Right(0).Top(0).Bottom(0).IgnoreInput(true).Opacity(0)
				}
				play.Children(func() { ui.Icon(c, theme.Icon("play")).FontSize(14).TextColor(ink) })
				if play.Clicked() && o.OnPlay != nil {
					o.OnPlay(i)
				}
			}
			if !shown {
				ui.Text(c, strconv.Itoa(number)).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
					TextColor(faint).FontFeatures("tnum")
			}
		})
		o.cell(ui.Row(c.Key("title")).Role(ui.RoleCell).Gap(12).AlignItems(ui.Center), cellTitle).Children(func() {
			if cols.Artwork {
				Artwork(c, o.Art, s.Art.Small, tk.Metric["track-art"], ArtOpts{})
			}
			ui.Column(c).MinWidth(0).Grow(1).Justify(ui.Center).Children(func() {
				ui.Row(c).MinWidth(0).Gap(6).AlignItems(ui.Center).Children(func() {
					t := ui.Text(c, title).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).
						TextColor(ink).SingleLine().Shrink(1).MinWidth(0)
					if current {
						t.FontWeight(500).TextColor(theme.C("play-ink"))
					}
					if s.Explicit {
						ExplicitMark(c.Key("explicit"))
					}
					if s.CatalogID != "" && o.Popular[s.CatalogID] {
						ui.Text(c, i18n.T("Popular")).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
							TextColor(faint).SingleLine().Shrink(0)
					}
				})
				if sub != "" {
					ui.Text(c, sub).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).
						TextColor(muted).SingleLine()
				}
			})
		})
		if cols.Artist {
			o.cell(ui.Row(c.Key("artist")).Role(ui.RoleCell).AlignItems(ui.Center), cellArtist).Children(func() {
				trackLink(c, w, s.Artist, s.ArtistID, muted, o.OpenArtist, selectRow)
			})
		}
		if cols.Album {
			o.cell(ui.Row(c.Key("album")).Role(ui.RoleCell).AlignItems(ui.Center), cellAlbum).Children(func() {
				trackLink(c, w, s.Album, s.AlbumID, muted, o.OpenAlbum, selectRow)
			})
		}
		if o.AddedBy != nil {
			o.cell(ui.Row(c.Key("by")).Role(ui.RoleCell).Gap(8).AlignItems(ui.Center), cellAddedBy).Children(func() {
				if p := o.AddedBy(i); p != nil {
					name := p.Name
					if name == "" {
						name = "@" + p.Handle
					}
					Artwork(c, o.Art, p.Art.Small, 20, ArtOpts{Round: true})
					trackLink(c, w, name, p.ID, muted, o.OpenProfile, selectRow)
				}
			})
		}
		dur := "—"
		if s.DurationMs > 0 {
			dur = i18n.FormatDuration(time.Duration(s.DurationMs) * time.Millisecond)
		}
		o.cell(ui.Row(c.Key("dur")).Role(ui.RoleCell).Justify(ui.End).AlignItems(ui.Center), cellDuration).Children(func() {
			ui.Text(c, dur).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).TextColor(faint).
				FontFeatures("tnum").SingleLine()
		})
		o.cell(ui.Row(c.Key("act")).Role(ui.RoleCell).Justify(ui.End).AlignItems(ui.Center), cellActions).Children(func() {
			if o.Menus != nil {
				o.Menus.Trigger(c.Key("more"), target, TriggerOpts{Visible: hot})
			}
		})
	})

	st.nav.Claim(src, i)
	if src.Clicked() {
		st.nav.SetFocus(i)
		if !src.Focused() {
			st.nav.Want(i)
		}
		st.Sel.Click(i, ModsOf(src.ClickModifiers()))
	}
	if src.DoubleClicked() && !disabled && o.OnPlay != nil {
		o.OnPlay(i)
	}
	if src.Focused() {
		if !st.nav.Keys(src, i, &st.Sel) && !st.Sel.Keys(src, i) && src.Shortcut(0, ui.KeyEnter) {
			if s.Playable && !withheld && o.OnPlay != nil {
				o.OnPlay(i)
			}
		}
	}
	if o.Menus != nil && key != "" {
		collectionsMenu(&st.Sel, o.Menus, src, target, i)
	}
}

// trackLink is an artist or album cell: a link to the page when the song
// names one (underlined while hovered; a click opens it without selecting
// the row), plain text otherwise.
func trackLink(c *ui.Context, row ui.Element, text, id string, col ui.Color, open func(id string), selectRow func(ClickMods)) {
	tk := theme.Current()
	t := ui.Text(c, text).FontSize(tk.Type["sub"].Size).FixedLineHeight(tk.Type["sub"].LineHeight).TextColor(col).SingleLine().MinWidth(0)
	if id == "" || open == nil {
		return
	}
	t.Role(ui.RoleLink)
	if t.Hovered() {
		t.Underline().TextColor(theme.C("ink")).Cursor(ui.CursorPointer)
	}
	if t.Clicked() {
		if m := ModsOf(t.ClickModifiers()); m.Range || m.Toggle {
			selectRow(m)
			return
		}
		open(id)
	}
}

// skeletonRow is a row whose page is still loading: flat bars where the
// text goes, busy for assistive technology.
func (st *TrackListState) skeletonRow(c *ui.Context, o *TrackListOpts, i int, hover ui.Color) {
	tk := theme.Current()
	r := tk.Radius["sm"]
	ui.Row(c.Key("loading-" + strconv.Itoa(i))).Height(TrackRowHeight).Shrink(0).PaddingX(trackPadX).Gap(trackGap).
		AlignItems(ui.Center).Role(ui.RoleRow).Label(i18n.T("Loading")).Children(func() {
		o.cell(ui.Box(c), cellNumber).Height(1)
		o.cell(ui.Row(c).Gap(12).AlignItems(ui.Center), cellTitle).Children(func() {
			if o.Columns.Artwork {
				ui.Box(c).Size(tk.Metric["track-art"], tk.Metric["track-art"]).Shrink(0).Radius(r).Background(hover)
			}
			ui.Box(c).WidthPercent(40).Height(14).Radius(r).Background(hover)
		})
		bar := func(kind int) {
			o.cell(ui.Row(c), kind).Children(func() { ui.Box(c).WidthPercent(50).Height(12).Radius(r).Background(hover) })
		}
		if o.Columns.Artist {
			bar(cellArtist)
		}
		if o.Columns.Album {
			bar(cellAlbum)
		}
		if o.AddedBy != nil {
			bar(cellAddedBy)
		}
		o.cell(ui.Box(c), cellDuration).Height(1)
		o.cell(ui.Box(c), cellActions).Height(1)
	})
}
