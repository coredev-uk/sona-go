package album

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry, from AppShell's main and AlbumPage (DIPs).
const (
	padTop    = 28 // pt-7
	padBottom = 48 // pb-12
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
	auth  authAPI
	queue *queueCommands
	// states is the library state, rating and the commands that change it.
	states *kit.ItemStates
	// signIn opens the account dialog (the "not signed in" error's button);
	// nil leaves the button out.
	signIn func()
	now    func() time.Time

	// menus are the item menus of the head and the shelves (no selection).
	menus *kit.ActionMenus

	stamp    time.Time
	reloaded string // the album whose release this page reloaded it for

	// pageState is the page being built: another album is another state, so
	// opening one from another starts at its top.
	*pageState
}

// pageState is the state of one album's page, kept on the page's root.
type pageState struct {
	pos          ui.ScrollState
	paneW, paneH float32 // the scroll container as last laid out
	// trackMenus are the songs' item menus, over their selection.
	trackMenus *kit.ActionMenus

	tracks  kit.TrackListState
	shelves [6]kit.CoverShelfState
	// The list as the last frame built it, for the disc index.
	list     listView
	gridTop  float32 // the wide layout's top in the pane's content
	gridH    float32
	sleeveH  float32
	paneRect func() ui.Rect
}

// listView is what the disc index needs of the track list.
type listView struct {
	opts  kit.TrackListOpts
	count int
	at    func(i int) *services.Song
}

// init follows the account: the sign-in state decides who may pre-add and
// the explicit policy (v0.2.0's auth module kept the query current).
func (a *album) init() {
	a.states.Follow(a.env.Bus)
	store.On(a.env.Bus, services.EventAuthChanged, func(s services.AuthStatus) {
		store.Set(a.env.Queries, keyAuth, s)
	})
}

// has reports whether a registered page matches path: the page links only
// to ported pages.
func (a *album) has(path string) bool { return a.env.HasRoute != nil && a.env.HasRoute(path) }

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
		key := catalogKey(id)
		r := store.Use(q, key, func(ctx context.Context) (services.CatalogAlbum, error) { return a.cat.Album(ctx, id) }, forever)
		l := load{hasData: r.HasData, err: r.Err, fetching: r.Fetching, key: key, cat: true}
		if r.HasData {
			l.detail, l.page = r.Data.AlbumDetail, &r.Data
		}
		return l
	}
	key := libraryKey(id)
	r := store.Use(q, key, func(ctx context.Context) (services.AlbumDetail, error) { return a.lib.Album(ctx, id) }, forever)
	l := load{hasData: r.HasData, err: r.Err, fetching: r.Fetching, key: key}
	if !r.HasData {
		return l
	}
	l.detail = r.Data
	if cid := r.Data.Album.CatalogID; cid != "" {
		linked := store.Use(q, catalogKey(cid), func(ctx context.Context) (services.CatalogAlbum, error) { return a.cat.Album(ctx, cid) }, forever)
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
	if st.trackMenus == nil {
		st.trackMenus = kit.NewActionMenus(a.env.Actions, &st.tracks.Sel)
	}

	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if st.paneW > 0 && st.paneW < 960 {
		gutter = tk.Metric["gutter-narrow"]
	}
	scrollbar := tk.Metric["scrollbar"]
	width := st.paneW
	if width == 0 {
		ww, _ := c.Size()
		width = ww - sidebarGuess
	}
	lay := albumLayout(width - 2*gutter - scrollbar)
	root.Children(func() { a.scroll(c, id, highlight, lay, now, gutter, scrollbar) })
	a.menus.Popup(c)
}

// scroll is the page's scroll container.
func (a *album) scroll(c *ui.Context, id, highlight string, lay layout, now time.Time, gutter, scrollbar float32) {
	var sc ui.Element
	devstate.Scroll(c, &a.pos) // dev builds: the offset survives a restart
	sc = ui.Scroll(c).TrackScroll(&a.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	a.paneRect = func() ui.Rect { return sc.Bounds() }
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Children(func() {
			a.content(c, id, highlight, lay, now)
		})
	})
	if b := sc.Bounds(); b.W != a.paneW || b.H != a.paneH {
		a.paneW, a.paneH = b.W, b.H
		c.Invalidate() // the layout follows the pane's width
	}
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
	// An album not out yet counts down to the minute; at its release the
	// page reloads it so its songs become playable.
	if al := l.detail.Album; al.Prerelease && !isUpcoming(al, now) {
		if a.reloaded != id {
			a.reloaded = id
			a.env.Queries.Invalidate(l.key)
		}
	} else if a.reloaded == id {
		a.reloaded = ""
	}
	auth := store.Use(a.env.Queries, keyAuth, func(context.Context) (services.AuthStatus, error) { return a.auth.Status(), nil }, forever)
	m := newModel(l.detail, l.page, l.cat, now)
	a.body(c, &m, lay, highlight, now, auth.Data)
}

// errorBlock is DetailError: not found, signed out, or the failure with
// Retry; the way back is the library's Albums.
func (a *album) errorBlock(c *ui.Context, l load) {
	kit.DetailError(c, kit.NounAlbum, l.err, kit.DetailErrorOpts{
		Retry:    func() { a.env.Queries.Invalidate(l.key) },
		Retrying: l.fetching,
		Back:     kit.EmptyAction{Label: i18n.T("Go to Albums"), Run: func() { a.push("/library/albums") }},
		SignIn:   a.signIn,
	})
}

// typed is text in one of the theme's type styles.
func typed(c *ui.Context, text, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, text).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// skeleton is the sleeve, head and rows at their real sizes while the album
// loads.
func (a *album) skeleton(c *ui.Context, lay layout) {
	hover := theme.C("hover")
	tk := theme.Current()
	bar := func(w, h float32) ui.Element {
		return ui.Box(c).Width(w).Height(h).Radius(tk.Radius["sm"]).Background(hover).Shrink(0)
	}
	cover := func() {
		ui.Box(c).Size(lay.cover, lay.cover).Radius(tk.Radius["sm"]).Background(hover).Shrink(0)
	}
	head := func() {
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Box(c).Height(32).WidthPercent(60).Radius(tk.Radius["sm"]).Background(hover)
			ui.Box(c).Height(16).WidthPercent(25).Radius(tk.Radius["sm"]).Background(hover).Margin(12, 0, 0, 0)
			ui.Box(c).Height(12).WidthPercent(33).Radius(tk.Radius["sm"]).Background(hover).Margin(8, 0, 0, 0)
			ui.Row(c).Gap(8).Margin(24, 0, 0, 0).Children(func() {
				ui.Box(c).Size(40, 40).Radius(tk.Radius["full"]).Background(hover)
				bar(96, 32).Radius(tk.Radius["md"])
			})
		})
	}
	rows := func() {
		for i := range 8 {
			ui.Row(c.Key(i)).FillWidth().Height(44).PaddingX(8).Gap(16).AlignItems(ui.Center).Children(func() {
				bar(20, 12)
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
