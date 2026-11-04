package playlist

import (
	"context"
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
	// states is the header's library state and rating, built on first use.
	states *headerState
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
	*pg = page{id: id, states: pg.states}
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

// headerStates are the header's library state and rating.
func (p *playlists) headerStates() *headerState {
	pg := &p.page
	if pg.states == nil && p.env.Core != nil {
		pg.states = newHeaderState(p.env.Queries, p.env.Post, services.Service[services.LibraryService](p.env.Core))
	}
	return pg.states
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
				kit.DetailSkeleton(c, false)
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

// playList plays the playlist from song start, or shuffled, off the main
// thread.
func (p *playlists) playList(id string, songs []services.Song, start int, shuffle bool) {
	from := services.PlayFrom{Kind: "playlist", ID: id}
	go p.playSongs(context.Background(), songs, start, from, shuffle)
}

// setEditing enters or leaves the edit mode; the focus moves to the button
// that replaces the pressed one.
func (pg *page) setEditing(c *ui.Context, on bool) {
	pg.editing, pg.focusToggle = on, true
	pg.editor = editor{}
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
	hd := d.Header
	cover := func(c *ui.Context, size float32) {
		kit.MotionCover(c, p.env.Motion, p.env.Art, pl.Art.Large, hd.Motion, size, kit.MotionOpts{VideoSize: 200})
	}
	if hd.Motion == "" {
		cover = nil
	}
	// The header's wash (kit.Backdrop) is left out until CollectionHeader's
	// Wash slot lands (kit.ItemStates' PR): its Backdrop slot draws over the
	// title and the actions.
	kit.CollectionHeader(c, kit.CollectionHeaderOpts{
		Title:       pl.Name,
		Art:         pl.Art.Large,
		Loader:      p.env.Art,
		Cover:       cover,
		Byline:      byline,
		Meta:        p.metaLine(d, collab),
		Description: pl.Description,
		Actions:     func() { p.headerActions(c, id, d, editable, inEdit, menus) },
	})

	switch {
	case inEdit && len(d.Tracks) > 0:
		p.editorList(c, id, d, pane)
	case inEdit:
		kit.EmptyState(c, i18n.T("Nothing left to arrange"),
			i18n.T("Add songs with “Add to playlist” in any song, album or playlist menu."), kit.EmptyOpts{})
	case len(d.Tracks) > 0:
		p.trackList(c, id, d, editable, menus, pane)
	default:
		text := i18n.T("Songs added to it in Apple Music on any device appear here after the next sync.")
		if editable {
			text = i18n.T("Add songs with “Add to playlist” in any song, album or playlist menu, or pick from the suggestions below.")
		}
		kit.EmptyState(c, i18n.T("This playlist is empty"), text, kit.EmptyOpts{})
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

// metaLine is the catalogue line: kind · "You're a collaborator" · song
// count and duration · "Updated <relative time>".
func (p *playlists) metaLine(d services.PlaylistDetail, collab *services.PlaylistCollaboration) string {
	kind := i18n.T("Playlist")
	switch {
	case collab != nil:
		kind = i18n.T("Collaborative playlist")
	case d.Kind == "personal-mix":
		kind = i18n.T("Made for you")
	case d.IsChart:
		kind = i18n.T("Chart")
	case d.Favorited:
		kind = i18n.T("Your favourites")
	}
	if collab != nil && collab.Viewer == services.RoleCollaborator {
		kind += " · " + i18n.T("You're a collaborator")
	}
	line := catalogueLine(kind, len(d.Tracks), d.DurationMs)
	if d.Playlist.LastModified > 0 {
		line += " · " + i18n.T("Updated {when}", "when", relativeTime(d.Playlist.LastModified, time.Now()))
	}
	return line
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
		if kit.Button(c.Key("details"), i18n.T("Name and description"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Clicked() {
			p.dialogs.show(dialogState{kind: dialogDetails, id: id, name: pl.Name, description: pl.Description, isPublic: d.IsPublic})
		}
		return
	}
	playable := false
	for _, s := range d.Tracks {
		if s.Playable {
			playable = true
			break
		}
	}
	play, shuffle := kit.PlayButtons(c, kit.PlayButtonsOpts{Label: pl.Name, Shuffle: true, Disabled: !playable})
	if play {
		p.playList(id, d.Tracks, 0, false)
	}
	if shuffle {
		p.playList(id, d.Tracks, 0, true)
	}
	if st := p.headerStates(); st != nil {
		if pl.Source == services.SourceCatalog {
			st.libraryButton(c, pl)
		}
		st.ratingButtons(c, pl)
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
// columns, and for a collaborative playlist who added each. An editable
// playlist gives its rows the playlist as their container, so their menus
// offer Remove from playlist.
func (p *playlists) trackList(c *ui.Context, id string, d services.PlaylistDetail, editable bool, menus *kit.ActionMenus, pane *kit.Pane) {
	pg := &p.page
	tracks := d.Tracks
	o := kit.TrackListOpts{
		Label:   i18n.T("Songs in {playlistName}", "playlistName", d.Playlist.Name),
		Columns: kit.TrackColumns{RowNumbers: true, Artwork: true, Artist: true, Album: true},
		OnPlay:  func(i int) { p.playList(id, tracks, i, false) },
		Art:     p.env.Art,
		Menus:   menus,
		NoPopup: true,
		Motion:  p.env.Motion,
		Pane:    pane,
	}
	if editable {
		o.Container = &kit.ActionContainer{Kind: "playlist", ID: id, Editable: true}
	}
	if p.env.Play != nil {
		if st := p.env.Play.State(); st.Current != nil {
			cur := st.Current
			o.Current = func(s *services.Song) bool { return kit.SongIsCurrent(s, cur.ID, cur.CatalogID) }
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
		if rows, some := pg.addedBy(tracks, *collab); some {
			o.AddedBy = func(i int) *services.SocialProfile { return rows[i] }
		}
	}
	kit.TrackList(c, &pg.list, len(tracks), func(i int) *services.Song { return &tracks[i] }, o)
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
	p.shelf(c, &p.page.featured, title, title, items, menus)
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
	p.shelf(c, &p.page.more, title, i18n.T("More playlists from this curator"), items, menus)
}

// shelf is a titled single row of cover tiles.
func (p *playlists) shelf(c *ui.Context, st *kit.CoverState, title, label string, items []kit.CoverItem, menus *kit.ActionMenus) {
	kit.Shelf(c, kit.ShelfOpts{Title: title}, func() {
		kit.CoverRow(c, st, items, kit.CoverRowOpts{CoverOpts: kit.CoverOpts{
			Label: label, Art: p.env.Art, Motion: p.env.Motion, Menus: menus, NoPopup: true,
		}})
	})
}
