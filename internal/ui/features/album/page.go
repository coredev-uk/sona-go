package album

import (
	"context"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry, from AppShell's main and AlbumPage (DIPs).
const (
	// stickyTop is how far below the pane's top the sleeve stays while the
	// songs scroll (sticky top-7).
	stickyTop = 28
	// sidebarGuess stands for the sidebar and its line before the page has
	// been measured, so the first frame is already at the right layout.
	sidebarGuess = 233
	// clockStep is how often the countdown is re-read: to the minute, never
	// a ticking counter.
	clockStep = time.Minute
)

// album is the module's state: the services and what is shared by every
// page of it. What belongs to one page (the scroll place, the track list, the
// shelves) is its pageState.
type album struct {
	env   *module.Env
	lib   libraryAPI
	cat   catalogAPI
	radio radioAPI
	// signIn opens the account dialog (the "not signed in" error's button);
	// nil leaves the button out.
	signIn func()
	now    func() time.Time

	// menus are the item menus of the head and the shelves (no selection).
	menus *kit.ActionMenus

	stamp     time.Time
	reloaded  string    // the album whose release this page reloaded it for,
	reloadAt  time.Time // and when it last asked
	announced string    // the album whose release the page has announced
	tries     int       // how many times it has asked again since
	// waiting is the album the page counted down to, and waitSongs the IDs of
	// its songs that weren't out then.
	waiting   string
	waitSongs map[string]bool
	// release is withCatalogueRelease's answer, kept until its input changes.
	release releaseMemo

	// pageState is the page being built: another album is another state, so
	// opening one from another starts at its top.
	*pageState
}

// pageState is the state of one album's page, kept on the page's root.
type pageState struct {
	// PageFrame is the scroll place and the scroll container as last laid out.
	kit.PageFrame
	// trackMenus are the songs' item menus, over their selection.
	trackMenus *kit.ActionMenus

	tracks  kit.TrackListState
	shelves [6]kit.CoverShelfState
	// The list as the last frame built it, for the disc index.
	list    listView
	sleeveH float32   // the wide layout's sleeve, as last laid out
	pane    *kit.Pane // the scroll container, as this frame builds it
	// pal is the cover's colours, built once per cover, theme and layout.
	pal palCache
}

// listView is what the disc index needs of the track list.
type listView struct {
	opts  kit.TrackListOpts
	count int
	at    func(i int) *services.Song
}

// push opens a page.
func (a *album) push(path string) { a.env.Nav.Push(path) }

// clock is the time the page counts down from: re-read every minute, and the
// view is asked back when the minute is up.
func (a *album) clock(c *ui.Context) time.Time {
	t := a.now()
	if a.stamp.IsZero() || t.Sub(a.stamp) >= clockStep || t.Before(a.stamp) {
		a.stamp = t
	}
	c.After(max(time.Millisecond, a.stamp.Add(clockStep).Sub(t)))
	return a.stamp
}

// forever: v0.2.0's queries are never stale (staleTime Infinity) until an
// event or Retry invalidates them.
var forever = store.Options{Stale: store.Forever}

// load is one album query, whatever its kind.
type load struct {
	detail   services.AlbumDetail
	page     *services.CatalogAlbum // Apple's page, when there is one
	cat      bool                   // detail is the catalogue page itself
	hasData  bool
	err      error
	fetching bool
	key      store.Key
}

// query is AlbumPage's data: a library id loads LibraryService.Album, a
// catalogue id CatalogService.Album, and a library album Apple Music also has
// loads its catalogue page for the sections.
func (a *album) query(id string) load {
	q := a.env.Queries
	if !isLibraryID(id) {
		key := store.CatalogAlbum(id)
		r := store.Use(q, key, func(ctx context.Context) (services.CatalogAlbum, error) { return a.cat.Album(ctx, id) }, forever)
		l := load{hasData: r.HasData, err: r.Err, fetching: r.Fetching, key: key, cat: true}
		if r.HasData {
			l.detail, l.page = r.Data.AlbumDetail, &r.Data
		}
		return l
	}
	key := store.LibraryAlbum(id)
	r := store.Use(q, key, func(ctx context.Context) (services.AlbumDetail, error) { return a.lib.Album(ctx, id) }, forever)
	l := load{hasData: r.HasData, err: r.Err, fetching: r.Fetching, key: key}
	if !r.HasData {
		return l
	}
	l.detail = r.Data
	if cid := r.Data.Album.CatalogID; cid != "" {
		linked := store.Use(q, store.CatalogAlbum(cid), func(ctx context.Context) (services.CatalogAlbum, error) { return a.cat.Album(ctx, cid) }, forever)
		if linked.HasData {
			l.page = &linked.Data
		}
	}
	return l
}

// page is AlbumPage: one scroll container holding the layout of the
// content's width.
func (a *album) page(c *ui.Context, r *ui.Route) {
	id, highlight := r.Param("id"), r.Query("song")
	now := a.clock(c)
	// One state per album: the root is keyed by it, so a page for another
	// album (and the old one while the router changes) has its own.
	root := ui.Column(c.Key("album:" + id)).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	st := *ui.Local(root, "page", func() *pageState { return &pageState{} })
	a.pageState = st
	kit.MenusIn(&st.trackMenus, a.env.Actions, &st.tracks.Sel)

	width := st.Width
	if width == 0 {
		ww, _ := c.Size()
		width = ww - sidebarGuess
	}
	lay := albumLayout(width - 2*st.Gutter() - theme.Current().Metric["scrollbar"])
	root.Children(func() {
		st.Scroll(c, "", func(pane *kit.Pane, _ float32) {
			a.pane = pane
			a.content(c, id, highlight, lay, now)
		})
	})
	a.menus.Popup(c)
}

// content is the body for the album's state: its skeleton, the error, or the
// album.
func (a *album) content(c *ui.Context, id, highlight string, lay layout, now time.Time) {
	l := a.query(id)
	switch {
	case l.err != nil && !l.hasData:
		a.errorBlock(c, l)
		return
	case !l.hasData:
		a.skeleton(c, lay)
		return
	}
	if !l.cat {
		l.detail = a.release.of(l.detail, l.page, now)
	}
	a.waitForSongs(&l.detail, id, now)
	// An album not out yet counts down to the minute; at its release the
	// page says so, announces it, and reloads the album so its songs become
	// playable, asking again every minute (at most maxWaitTries times) until
	// Apple's data catches up.
	if al := l.detail.Album; al.Prerelease && !kit.IsUpcoming(al, now) {
		if a.announced != id {
			a.announced = id
			c.Announce(i18n.T("{name} is out now", "name", al.Name))
		}
		switch {
		case a.reloaded != id:
			a.reloaded, a.reloadAt, a.tries = id, now, 1
			a.env.Queries.Invalidate(l.key)
		case now.Sub(a.reloadAt) >= clockStep && a.tries < maxWaitTries:
			a.reloadAt, a.tries = now, a.tries+1
			a.env.Queries.Invalidate(l.key)
		}
	} else if a.reloaded == id {
		a.reloaded, a.announced, a.tries = "", "", 0
	}
	auth := store.UseAuth(a.env.Queries, a.env.Core)
	m := newModel(l.detail, l.page, l.cat, now)
	a.body(c, &m, lay, highlight, now, auth.Data)
}

// maxWaitTries is how many times a page asks again, a minute apart, for the
// songs of an album that has come out.
const maxWaitTries = 30

// waitForSongs keeps an album that has come out on a page that counted down
// to it "just out" while the songs that were waiting are still unplayable.
// The service derives "not a prerelease" from the date alone, so once the
// release time has passed it says the album is out whether or not Apple has
// published the songs (they have no play parameters until it does): such an
// album is marked a prerelease again for the page, which then asks again
// (maxWaitTries times) instead of showing every song "Unavailable". An album
// the page didn't see upcoming is shown as it is.
func (a *album) waitForSongs(d *services.AlbumDetail, id string, now time.Time) {
	switch {
	case kit.IsUpcoming(d.Album, now):
		if a.waiting != id {
			a.waiting, a.waitSongs = id, map[string]bool{}
			for _, s := range d.Tracks {
				if !s.Playable {
					a.waitSongs[s.ID] = true
				}
			}
		}
	case a.waiting != id || d.Album.Prerelease:
	case a.tries < maxWaitTries && slices.ContainsFunc(d.Tracks, func(s services.Song) bool { return !s.Playable && a.waitSongs[s.ID] }):
		d.Album.Prerelease = true
	default:
		a.waiting, a.waitSongs = "", nil
	}
}

// errorBlock is DetailError: not found, signed out, or the failure with
// Retry; the way back is the library's Albums.
func (a *album) errorBlock(c *ui.Context, l load) {
	res := store.Result[struct{}]{Err: l.err, Fetching: l.fetching}
	back := kit.BackAction(i18n.T("Go to Albums"), "/library/albums", true, a.push)
	kit.DetailQueryError(c, kit.NounAlbum, a.env.Queries, res, back, a.signIn, l.key)
}

// skeleton is the sleeve, head and rows at their real sizes while the album
// loads.
func (a *album) skeleton(c *ui.Context, lay layout) {
	hover := theme.C("hover")
	tk := theme.Current()
	cover := func() { kit.Bar(c, lay.cover, lay.cover) }
	head := func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Box(c).Height(32).WidthPercent(60).Radius(tk.Radius["sm"]).Background(hover)
			ui.Box(c).Height(16).WidthPercent(25).Radius(tk.Radius["sm"]).Background(hover).Margin(12, 0, 0, 0)
			ui.Box(c).Height(12).WidthPercent(33).Radius(tk.Radius["sm"]).Background(hover).Margin(8, 0, 0, 0)
			ui.Row(c).Gap(8).Margin(24, 0, 0, 0).Children(func() {
				ui.Box(c).Size(40, 40).Radius(tk.Radius["full"]).Background(hover)
				kit.Bar(c, 96, 32).Radius(tk.Radius["md"])
			})
		})
	}
	rows := func() {
		for i := range 8 {
			ui.Row(c.Key(i)).FillWidth().Height(44).PaddingX(8).Gap(16).AlignItems(ui.Center).Children(func() {
				kit.Bar(c, 20, 12)
				ui.Box(c).Height(14).WidthPercent(float32(30 + (i*17)%30)).Radius(tk.Radius["sm"]).Background(hover)
			})
		}
	}
	root := ui.Column(c).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading"))
	root.Children(func() {
		if lay.wide {
			ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(40).Children(func() {
				ui.Column(c).Width(lay.cover + 32).Padding(16).Shrink(0).Children(cover)
				ui.Column(c).Grow(1).MinWidth(0).Padding(16, 0, 0, 0).Children(func() {
					head()
					ui.Column(c).Margin(24, 0, 0, 0).FillWidth().Children(rows)
				})
			})
			return
		}
		ui.Row(c).FillWidth().Gap(20).Padding(20).AlignItems(ui.Start).Children(func() {
			cover()
			head()
		})
		ui.Column(c).Margin(24, 0, 0, 0).FillWidth().Children(rows)
	})
}
