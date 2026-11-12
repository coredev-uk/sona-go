package playlist

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// narrowPage is the window width under which the page's gutters shrink.
const narrowPage = 960

// page is the playlist page's view state. Everything but states belongs to
// the playlist shown: opening another starts it afresh.
type page struct {
	id  string
	pos ui.ScrollState
	// list is the track list's selection and navigation; menus the item
	// menus of the page's lists and header.
	list  kit.TrackListState
	menus *kit.ActionMenus
	// editing is the edit mode; focusToggle moves the focus to the button
	// that replaced the pressed Edit or Done.
	editing     bool
	focusToggle bool
	editor      editor
	collab      collaborators
	suggest     suggestState
	featured    kit.CoverState
	more        kit.CoverState
	// added caches addedByRows for the tracks and attributions it was made
	// from (the rows are rebuilt only when either is another list).
	added addedCache
	// sort and query are how the songs are sorted and narrowed (no key: the
	// playlist's own order); shownCache is what they make of the tracks, and
	// announced the filter's result last read out.
	sort       kit.TrackSortState
	query      string
	shownCache viewCache
	// startsCache is the "starts at" labels of the tracks.
	startsCache startsCache
	announced   string
}

// addedCache is addedByRows' last answer and what it came from.
type addedCache struct {
	tracks *services.Song
	nT     int
	by     *services.TrackAddedBy
	nBy    int
	rows   []*services.SocialProfile
	some   bool
}

// addedBy is addedByRows, kept while neither the tracks nor Apple's
// attributions change (the page asks every frame).
func (pg *page) addedBy(tracks []services.Song, c services.PlaylistCollaboration) ([]*services.SocialProfile, bool) {
	a := &pg.added
	var t *services.Song
	if len(tracks) > 0 {
		t = &tracks[0]
	}
	var by *services.TrackAddedBy
	if len(c.AddedBy) > 0 {
		by = &c.AddedBy[0]
	}
	if a.rows == nil || a.tracks != t || a.nT != len(tracks) || a.by != by || a.nBy != len(c.AddedBy) {
		rows := addedByRows(tracks, c)
		a.tracks, a.nT, a.by, a.nBy, a.rows, a.some = t, len(tracks), by, len(c.AddedBy), rows, false
		for _, r := range rows {
			if r != nil {
				a.some = true
				break
			}
		}
		if a.rows == nil {
			a.rows = []*services.SocialProfile{}
		}
	}
	return a.rows, a.some
}

// enter makes id the playlist the page shows: another playlist starts from
// its top, out of edit mode.
func (pg *page) enter(id string) {
	if pg.id == id {
		return
	}
	*pg = page{id: id}
}

// hasRoute reports whether a page of Sona answers path, so a link is drawn
// only to pages that exist.
func (p *playlists) hasRoute(path string) bool {
	return p.env.HasRoute != nil && p.env.HasRoute(path)
}

// itemMenus are the page's action menus, once the shell has its actions.
func (p *playlists) itemMenus() *kit.ActionMenus {
	pg := &p.page
	if pg.menus == nil && p.env.Actions != nil {
		pg.menus = kit.NewActionMenus(p.env.Actions, &pg.list.Sel)
	}
	return pg.menus
}

// playlistPage is PlaylistPage: a catalogue or library playlist by the
// route's id. The whole page scrolls in one container, header included.
func (p *playlists) playlistPage(c *ui.Context, r *ui.Route) {
	id := r.Param("id")
	pg := &p.page
	pg.enter(id)
	q := p.detail(id)

	tk := theme.Current()
	ww, _ := c.Size()
	gutter := tk.Metric["gutter"]
	if ww < narrowPage {
		gutter = tk.Metric["gutter-narrow"]
	}
	devstate.Scroll(c, &pg.pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(28, gutter+tk.Metric["scrollbar"], 48, gutter).Children(func() {
			switch {
			case !q.HasData && q.Err != nil:
				p.loadError(c, id, q)
			case !q.HasData:
				p.pageSkeleton(c)
			default:
				pane := &kit.Pane{Pos: &pg.pos, Bounds: func() ui.Rect { return sc.Bounds() }}
				p.playlistBody(c, id, q.Data, pane)
			}
		})
	})
}

// loadError is the page when the playlist could not be had: not found with
// a way back to the library's playlists, otherwise Try again.
func (p *playlists) loadError(c *ui.Context, id string, q store.Result[services.PlaylistDetail]) {
	back := kit.EmptyAction{Label: i18n.T("Go to Playlists")}
	if p.hasRoute(playlistsPath) {
		back.Run = func() { p.env.Nav.Push(playlistsPath) }
	}
	kit.DetailError(c, kit.NounPlaylist, q.Err, kit.DetailErrorOpts{
		Retry:    func() { p.env.Queries.Invalidate(keyDetail(id)) },
		Retrying: q.Fetching,
		Back:     back,
	})
}

// collabOf is the playlist's collaboration when it has one: only user
// playlists shared to Apple Music (pl.u-…) are asked.
func (p *playlists) collabOf(id string, pl services.Playlist) *services.PlaylistCollaboration {
	cid := pl.CatalogID
	if cid == "" {
		cid = id
	}
	if !strings.HasPrefix(cid, "pl.u-") {
		return nil
	}
	if res := p.collaboration(id); res.HasData && res.Data.Collaborative {
		return &res.Data
	}
	return nil
}

// playList plays songs, the playlist as the page lists them (sorted, and
// narrowed by the filter), from song start, or shuffled. Main thread.
func (p *playlists) playList(id string, songs []services.Song, start int, shuffle bool) {
	from := services.PlayFrom{Kind: "playlist", ID: id}
	p.playSongs(songs, start, from, shuffle)
}

// setEditing enters or leaves the edit mode; the focus moves to the button
// that replaces the pressed one.
func (pg *page) setEditing(c *ui.Context, on bool) {
	pg.editing, pg.focusToggle = on, true
	pg.editor = editor{}
	// The editor arranges the playlist itself, in its own order and whole.
	pg.clearView()
	c.Invalidate()
}

// playlistBody is the loaded page: the header, the songs (or the editor, or
// an empty state) and the shelves.
func (p *playlists) playlistBody(c *ui.Context, id string, d services.PlaylistDetail, pane *kit.Pane) {
	pg := &p.page
	pl := d.Playlist
	// A playlist is editable only when it is a library playlist with canEdit.
	editable := d.Kind == "library" && pl.CanEdit
	if pg.editing && !editable {
		pg.editing = false
	}
	inEdit := editable && pg.editing
	collab := p.collabOf(id, pl)
	menus := p.itemMenus()

	var byline func()
	switch {
	case collab != nil:
		byline = func() { p.collaboratorsByline(c, *collab) }
	case curatorName(d) != "":
		byline = func() { p.byline(c, d) }
	}
	note, detail := p.updated(c, pl)
	var find func()
	if !inEdit && len(d.Tracks) > 0 {
		find = func() { p.songFinder(c, d.Tracks) }
	}
	p.header(c, d, byline, p.ledger(d, collab, note, detail), func() { p.headerActions(c, id, d, editable, inEdit, menus) }, find)

	switch {
	case inEdit && len(d.Tracks) > 0:
		p.editorList(c, id, d, pane)
	case inEdit:
		emptyBlock(c, i18n.T("Nothing left to arrange"),
			i18n.T("Add songs with “Add to playlist” in any song, album or playlist menu."))
	case len(d.Tracks) > 0:
		p.trackList(c, id, d, editable, menus, pane)
	default:
		text := i18n.T("Songs added to it in Apple Music on any device appear here after the next sync.")
		if editable {
			text = i18n.T("Add songs with “Add to playlist” in any song, album or playlist menu, or pick from the suggestions below.")
		}
		emptyBlock(c, i18n.T("This playlist is empty"), text)
	}

	if editable && !inEdit {
		p.suggestionsShelf(c, id, pl.Name)
	}
	p.featuredShelf(c, d, menus)
	p.moreShelf(c, d, menus)
	if menus != nil {
		menus.Popup(c)
	}
}

// curatorName is who made the playlist, "" when nobody is named.
func curatorName(d services.PlaylistDetail) string {
	if d.Curator.Name != "" {
		return d.Curator.Name
	}
	return d.Playlist.Curator
}

// byline is who made the playlist: a link to the curator's or profile's page
// when Sona has one, else plain text.
func (p *playlists) byline(c *ui.Context, d services.PlaylistDetail) {
	cur := d.Curator
	name := curatorName(d)
	path := ""
	switch {
	case cur.ID != "" && (cur.Kind == "apple-curator" || cur.Kind == "curator"):
		path = "/curator/" + cur.ID
	case cur.ID != "" && cur.Kind == "profile":
		path = "/profile/" + cur.ID
	}
	if path != "" && p.hasRoute(path) {
		ui.Link(c.Key("curator"), name, path).TextColor(theme.C("ink")).SingleLine()
		return
	}
	ui.Text(c, name).TextColor(theme.C("ink-muted")).SingleLine()
}

// updated is when the playlist last changed: "Updated yesterday" in
// calendar words (i18n.RelativeDay), and the exact date and time behind it
// ("Monday, November 9, 2026 at 9:30 PM");
// "" when Apple gives no date. The page is asked back when the words would
// change: at the next local midnight, and every minute while it changed
// today.
func (p *playlists) updated(c *ui.Context, pl services.Playlist) (note, detail string) {
	if pl.LastModified <= 0 {
		return "", ""
	}
	now := c.Now()
	if p.now != nil {
		now = p.now()
	}
	t := time.UnixMilli(pl.LastModified).In(time.Local)
	wake := i18n.NextMidnight(now, time.Local).Sub(now)
	if i18n.CalendarDays(t, now, time.Local) <= 0 {
		wake = min(wake, time.Minute)
	}
	c.After(max(time.Millisecond, wake))
	return i18n.T("Updated {when}", "when", i18n.RelativeDay(t, now, time.Local)), i18n.FormatDateTime(t)
}

// headerActions are the header's controls: Play and Shuffle, the library
// button of a catalogue playlist, the ratings, Edit and the item menu; in edit
// mode Done and "Name and description" replace them.
func (p *playlists) headerActions(c *ui.Context, id string, d services.PlaylistDetail, editable, inEdit bool, menus *kit.ActionMenus) {
	pg := &p.page
	pl := d.Playlist
	if inEdit {
		done := kit.Button(c.Key("done"), i18n.T("Done"), kit.ButtonOpts{Size: kit.SizeSM, Icon: "check"})
		if pg.focusToggle {
			pg.focusToggle = false
			done.Focus()
		}
		if done.Clicked() {
			pg.setEditing(c, false)
		}
		if kit.Button(c.Key("details"), i18n.T("Name and description"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).HasPopup(ui.PopupDialog).Clicked() {
			p.dialogs.show(dialogState{kind: dialogDetails, id: id, name: pl.Name, description: pl.Description, isPublic: d.IsPublic})
		}
		if kit.Button(c.Key("cover"), i18n.T("Cover"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "image"}).HasPopup(ui.PopupDialog).Clicked() {
			p.changeCover(id, pl.Name)
		}
		return
	}
	// Play and Shuffle take the songs as the page lists them.
	songs := pg.view(d.Tracks).songs
	playable := false
	for _, s := range songs {
		if s.Playable {
			playable = true
			break
		}
	}
	play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: pl.Name, Shuffle: true, Disabled: !playable})
	if play {
		p.playList(id, songs, 0, false)
	}
	if shuffle {
		p.playList(id, songs, 0, true)
	}
	if st := p.env.ItemStates; st != nil {
		item := kit.StateOfPlaylist(pl)
		if pl.Source == services.SourceCatalog {
			st.LibraryButton(c, kit.StatePlaylists, item, pl.Name, "")
		}
		st.RatingButtons(c, kit.StatePlaylists, item, pl.Name)
	}
	if editable {
		edit := kit.Button(c.Key("edit"), i18n.T("Edit"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "pencil"})
		if pg.focusToggle {
			pg.focusToggle = false
			edit.Focus()
		}
		if edit.Clicked() {
			pg.setEditing(c, true)
		}
	}
	if menus != nil {
		menus.Trigger(c, kit.ActionTarget{Item: kit.PlaylistItem(pl), Name: pl.Name}, kit.TriggerOpts{Header: true})
	}
}

// trackList is the playlist's songs: number, artwork, artist and album
// columns, and for a collaborative playlist who added each, under a field to
// find a song in it. The headers sort the songs and the field narrows them;
// both are only how the page lists them, so Play, Remove and the "Added by"
// column go through the songs' places in the playlist. An editable playlist
// gives its rows the playlist as their container, so their menus offer
// Remove from playlist.
func (p *playlists) trackList(c *ui.Context, id string, d services.PlaylistDetail, editable bool, menus *kit.ActionMenus, pane *kit.Pane) {
	pg := &p.page
	// The finder in the header's strip was built first: a key typed in it
	// changes the query, and the songs listed, their count and what is
	// announced all follow that query.
	v := pg.view(d.Tracks)
	songs := v.songs
	if len(songs) == 0 {
		theme.Text(c, i18n.T("No song in this playlist matches “{query}”.", "query", pg.query), "sub").
			TextColor(theme.C("ink-muted")).Padding(16, 8)
		return
	}
	pg.sort.Duration = true
	pg.sort.OnSort = func(key string) {
		pg.sort.Toggle(key)
		c.Announce(sortAnnouncement(pg.sort))
		c.Invalidate()
	}
	o := kit.TrackListOpts{
		Label:   i18n.T("Songs in {playlistName}", "playlistName", d.Playlist.Name),
		Columns: kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true},
		OnPlay:  func(i int) { p.playList(id, songs, i, false) },
		Sort:    &pg.sort,
		Art:     p.env.Art,
		Menus:   menus,
		NoPopup: true,
		Motion:  p.env.Motion,
		Pane:    pane,
	}
	// "Starts at" is the song's place on the tape: only true while the list
	// is in the playlist's own order, so a sort or a filter hides it.
	if v.order == nil {
		if sc := pg.starts(d.Tracks); sc.byRow != nil {
			o.Subline = func(s *services.Song) string {
				if at, ok := sc.byRow[s]; ok {
					return i18n.T("Starts at {time}", "time", at)
				}
				return ""
			}
		}
	}
	if editable {
		o.Container = &kit.ActionContainer{Kind: "playlist", ID: id, Editable: true, Order: v.order}
	}
	if p.env.Play != nil {
		if st := p.env.Play.State(); st.Current != nil {
			o.Current = st.Current
			o.Playing = st.State == "playing"
		}
	}
	if p.hasRoute("/artist/x") {
		o.OpenArtist = func(aid string) { p.env.Nav.Push("/artist/" + aid) }
	}
	if p.hasRoute("/album/x") {
		o.OpenAlbum = func(aid string) { p.env.Nav.Push("/album/" + aid) }
	}
	if p.hasRoute("/profile/x") {
		o.OpenProfile = func(pid string) { p.env.Nav.Push("/profile/" + pid) }
	}
	if collab := p.collabOf(id, d.Playlist); collab != nil {
		if rows, some := pg.addedBy(d.Tracks, *collab); some {
			o.AddedBy = func(i int) *services.SocialProfile { return rows[v.place(i)] }
		}
	}
	kit.TrackList(c, &pg.list, len(songs), func(i int) *services.Song { return &songs[i] }, o)
}

// songFinder is the field at the right end of the header's strip, "Find in
// playlist", and, while it holds a query, how many of the playlist's songs
// match it. Escape in the field clears it.
func (p *playlists) songFinder(c *ui.Context, tracks []services.Song) {
	pg := &p.page
	// A reversed row: the field is built first (the count asks for the
	// query it now holds) and sits at the row's right end, the count to its
	// left.
	ui.Row(c.Key("find")).Reverse().Gap(12).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Column(c.Key("field")).Shrink(0).Role(ui.RoleSearch).Label(i18n.T("Find songs in this playlist")).Children(func() {
			label := i18n.T("Find in playlist")
			kit.SearchField(c, &pg.query, kit.SearchFieldOpts{Placeholder: label, A11yLabel: label, Width: 240})
		})
		found := len(pg.view(tracks).songs)
		if pg.filtering() {
			theme.Text(c, i18n.T("{found} of {total, plural, one {# song} other {# songs}}",
				"found", i18n.FormatNumber(float64(found)), "total", len(tracks)), "sub").
				TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine()
		}
		p.announceFound(c, found)
	})
}

// announceFound reads out the filter's result when it changes (a filter's
// result is not otherwise announced).
func (p *playlists) announceFound(c *ui.Context, found int) {
	pg := &p.page
	text := ""
	if pg.filtering() {
		if found > 0 {
			text = i18n.T("{matches, plural, one {# song matches} other {# songs match}}", "matches", found)
		} else {
			text = i18n.T("No song matches “{query}”", "query", pg.query)
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

// sortAnnouncement says the order the songs are in now, as a click on a
// header changed it.
func sortAnnouncement(s kit.TrackSortState) string {
	var by string
	switch s.Key {
	case kit.SortTitle:
		by = i18n.T("Title")
	case kit.SortArtist:
		by = i18n.T("Artist")
	case kit.SortAlbum:
		by = i18n.T("Album")
	case kit.SortDuration:
		by = i18n.T("Duration")
	default:
		return i18n.T("Songs in playlist order")
	}
	if s.Desc {
		return i18n.T("Songs sorted by {column}, descending", "column", by)
	}
	return i18n.T("Songs sorted by {column}, ascending", "column", by)
}

// featuredShelf is the playlist's featured artists as round tiles linking to
// their pages.
func (p *playlists) featuredShelf(c *ui.Context, d services.PlaylistDetail, menus *kit.ActionMenus) {
	if len(d.FeaturedArtists) == 0 {
		return
	}
	linked := p.hasRoute("/artist/x")
	items := make([]kit.CoverItem, len(d.FeaturedArtists))
	for i, a := range d.FeaturedArtists {
		it := kit.ArtistItem(a)
		items[i] = kit.CoverItem{Key: a.ID, Title: a.Name, Art: a.Art.Medium, Round: true, Motion: a.Motion, Item: &it}
		if linked {
			id := a.ID
			items[i].Open = func() { p.env.Nav.Push("/artist/" + id) }
		}
	}
	title := i18n.T("Featured artists")
	p.shelf(c, &p.page.featured, title, title, featuredTile, items, menus)
}

// moreShelf is the curator's other playlists ("More like this" when nobody
// is named) linking to their pages.
func (p *playlists) moreShelf(c *ui.Context, d services.PlaylistDetail, menus *kit.ActionMenus) {
	if len(d.MoreByCurator) == 0 {
		return
	}
	linked := p.hasRoute(playlistPath)
	items := make([]kit.CoverItem, len(d.MoreByCurator))
	for i, m := range d.MoreByCurator {
		it := kit.PlaylistItem(m)
		items[i] = kit.CoverItem{Key: m.ID, Title: m.Name, Subtitle: m.Curator, Art: m.Art.Medium, Motion: m.Motion, Item: &it}
		if linked {
			id := m.ID
			items[i].Open = func() { p.openPlaylist(id) }
		}
	}
	title := i18n.T("More like this")
	if name := d.Curator.Name; name != "" {
		title = i18n.T("More from {curatorName}", "curatorName", name)
	}
	p.shelf(c, &p.page.more, title, i18n.T("More playlists from this curator"), moreTile, items, menus)
}

// The shelves' smallest tiles: the featured artists are a row of small
// portraits, the curator's playlists are covers.
const (
	featuredTile = 100
	moreTile     = 144
)

// shelf is a titled single row of cover tiles, minTile wide at least.
func (p *playlists) shelf(c *ui.Context, st *kit.CoverState, title, label string, minTile float32, items []kit.CoverItem, menus *kit.ActionMenus) {
	kit.Shelf(c, kit.ShelfOpts{Title: title}, func() {
		kit.CoverRow(c, st, items, kit.CoverRowOpts{CoverOpts: kit.CoverOpts{
			Min: minTile, Label: label, Art: p.env.Art, Motion: p.env.Motion, Menus: menus, NoPopup: true,
		}})
	})
}
