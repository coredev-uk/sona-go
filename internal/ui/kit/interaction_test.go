package kit

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The interaction-consistency review (#311) pins what each shared component
// does today for each gesture and each kind. The rule being reviewed: choosing
// a result opens it; Play plays; a track list's row plays (double-click or
// Enter); a modifier never does something a visible cue does not hint at.
// Where today's behaviour departs from it, the assertion carries a
// "DEPARTS:" comment, to be found (and flipped) when the behaviour is fixed.

// iaRec is the recorder of the real app's players: what discover registers
// (songs, albums, playlists, music videos, a song list) and radio (stations).
// Artists and curators have no player, so they have no Play control; the
// recorder of the other items tests registers every kind.
func iaRec() *itemsRec {
	r := newItemsRec()
	p := &ItemPlayers{
		Capabilities: func() *services.Capabilities {
			return &services.Capabilities{Playback: services.PlaybackCapabilities{Video: r.hasVideo}}
		},
		Now: func() time.Time { return r.clock },
	}
	for _, kind := range []string{services.ItemSong, services.ItemAlbum, services.ItemPlaylist, services.ItemStation} {
		p.Register(kind, func(it services.Item) { r.played = append(r.played, it.Kind+":"+it.ID) })
	}
	p.RegisterSongs(func(songs []services.Song, start int, label string) {
		names := make([]string, len(songs))
		for i, s := range songs {
			names[i] = s.Name
		}
		r.songs = append(r.songs, label+"@"+songs[start].Name+"/"+names[0]+".."+names[len(names)-1])
	})
	p.RegisterVideo(func(v services.MusicVideo, _ *ui.Handle) { r.videos = append(r.videos, v.ID) })
	r.players = p
	return r
}

// iaFx is what the gestures since iaReset asked of the app, in one line:
// "nothing" when none did.
func iaFx(r *itemsRec) string {
	var out []string
	add := func(name string, v []string) {
		if len(v) > 0 {
			out = append(out, name+"="+strings.Join(v, ","))
		}
	}
	add("nav", r.nav)
	add("opened", r.opened)
	add("played", r.played)
	add("songs", r.songs)
	add("videos", r.videos)
	add("urls", r.urls)
	if r.toggles > 0 {
		out = append(out, fmt.Sprintf("toggles=%d", r.toggles))
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, " ")
}

func iaReset(r *itemsRec) {
	r.nav, r.urls, r.opened, r.played, r.songs, r.videos, r.toggles = nil, nil, nil, nil, nil, nil, 0
}

// iaMenus is an item menu for every kind that has one: the tests only ask
// whether a right-click opens it ("Do something").
func iaMenus() *ActionMenus {
	var defs []ActionDef
	for _, k := range itemKinds {
		defs = append(defs, ActionDef{
			ID: "test." + string(k), Target: k, Group: GroupItem,
			Label: func(*ActionContext) string { return "Do something" },
			Run:   func(*ActionContext) {},
		})
	}
	return NewActionMenus(NewActionRegistry(defs...), nil)
}

// iaTab presses Tab until the control called label has the focus, and reports
// whether it got it.
func iaTab(tt *ui.Tester, label string) bool {
	for range 40 {
		if tt.Focused(label) {
			return true
		}
		tt.Key(0, ui.KeyTab)
	}
	return false
}

// iaRole names what the control called label is on the accessibility tree:
// "link", "button", "group", "list item", "row" or "" when none of those has
// the name.
func iaRole(tt *ui.Tester, label string) string {
	for _, r := range []struct {
		role ui.Role
		name string
	}{{ui.RoleLink, "link"}, {ui.RoleButton, "button"}, {ui.RoleGroup, "group"}, {ui.RoleListItem, "list item"}, {ui.RoleRow, "row"}} {
		if _, ok := tt.AccessByRole(r.role, label); ok {
			return r.name
		}
	}
	return ""
}

// iaDone is iaFx with the number of selected tiles or rows, when some are.
func iaDone(r *itemsRec, selected int) string {
	s := iaFx(r)
	switch {
	case selected == 0:
	case s == "nothing":
		s = fmt.Sprintf("sel=%d", selected)
	default:
		s += fmt.Sprintf(" sel=%d", selected)
	}
	return s
}

// iaRow is what each gesture did to one item of a shared component, and what
// the item looked like: the review's table, one row per surface and kind.
type iaRow struct {
	// role is what the item's opener is on the accessibility tree (link,
	// button, group); play is "disc" when it has a Play control of its own;
	// more is "button" when it has a "More for" button; menu is "opens" when
	// a right-click opens its menu.
	role, play, more, menu string
	// What the gestures asked of the app ("nothing" for none), with the number
	// of selected items when any.
	click, double, enter, shiftEnter, space string
	shiftClick, ctrlClick, playDisc         string
}

// iaSurface is a component built over one item, for iaDrive to drive.
type iaSurface struct {
	w, h int
	// mk builds a fresh page: what it builds in the context, and how many
	// items are selected (nil: the surface does not select).
	mk func() (build func(c *ui.Context, ctx ItemsCtx), selected func() int)
	// name is what the item is called; opener the name of its opener, a stop
	// of Tab; play the name of its Play control ("" for none to look for).
	name, opener, play string
	// at is where a pointer goes to choose the item.
	at func(tt *ui.Tester) (x, y float32)
	// tweak changes the page's context (what is playing, a route that is not
	// there).
	tweak func(*ItemsCtx)
}

// iaDrive drives the surface with each gesture, each on a fresh page, and
// reports what they did.
func iaDrive(t *testing.T, s iaSurface) iaRow {
	t.Helper()
	fresh := func() (*itemsApp, *ui.Tester, func() int) {
		build, selected := s.mk()
		a, tt := newItemsApp(t, s.w, s.h, nil, build)
		a.rec, a.tweak, a.menus = iaRec(), s.tweak, iaMenus()
		a.load(t, tt, 0)
		if selected == nil {
			selected = func() int { return 0 }
		}
		return a, tt, selected
	}
	gesture := func(do func(tt *ui.Tester)) string {
		a, tt, selected := fresh()
		do(tt)
		return iaDone(a.rec, selected())
	}
	key := func(mods ui.Modifiers, k ui.Key) string {
		a, tt, selected := fresh()
		if !iaTab(tt, s.opener) {
			return "not focusable"
		}
		tt.Key(mods, k)
		return iaDone(a.rec, selected())
	}
	click := func(mods ui.Modifiers, times int) string {
		return gesture(func(tt *ui.Tester) {
			x, y := s.at(tt)
			for range times {
				tt.ClickAtWith(mods, x, y)
			}
		})
	}
	var got iaRow
	_, tt, _ := fresh()
	got.role = iaRole(tt, s.opener)
	got.play = "none"
	if _, ok := tt.Find(s.play); ok && s.play != "" {
		got.play = "disc"
	}
	got.more = "none"
	if _, ok := tt.Find("More for " + s.name); ok {
		got.more = "button"
	}
	got.menu = "none"
	{
		_, tt, _ := fresh()
		x, y := s.at(tt)
		tt.RightClickAt(x, y)
		tt.Frame()
		if tt.HasText("Do something") {
			got.menu = "opens"
		}
	}
	got.click = click(0, 1)
	got.double = click(0, 2)
	got.enter = key(0, ui.KeyEnter)
	got.shiftEnter = key(ui.Shift, ui.KeyEnter)
	got.space = key(0, ui.KeySpace)
	got.shiftClick = click(ui.Shift, 1)
	got.ctrlClick = click(ui.Cmd, 1)
	got.playDisc = "no control"
	if got.play == "disc" {
		got.playDisc = gesture(func(tt *ui.Tester) {
			if err := tt.Click(s.play); err != nil {
				t.Fatal(err)
			}
		})
	}
	return got
}

// iaDiff reports each gesture of got that is not what was pinned.
func iaDiff(t *testing.T, name string, got, want iaRow) {
	t.Helper()
	check := func(what, g, w string) {
		if g != w {
			t.Errorf("%s: %s: got %q, want %q", name, what, g, w)
		}
	}
	check("role", got.role, want.role)
	check("play", got.play, want.play)
	check("more", got.more, want.more)
	check("menu", got.menu, want.menu)
	check("click", got.click, want.click)
	check("double-click", got.double, want.double)
	check("Enter", got.enter, want.enter)
	check("Shift+Enter", got.shiftEnter, want.shiftEnter)
	check("Space", got.space, want.space)
	check("Shift+click", got.shiftClick, want.shiftClick)
	check("Ctrl+click", got.ctrlClick, want.ctrlClick)
	check("Play", got.playDisc, want.playDisc)
}

// iaUnderTitle is the point on a tile's title row, under the cover and clear
// of its play control: the tile's opener (every tile of the tests has the
// subtitle "sub").
func iaUnderTitle(tt *ui.Tester) (float32, float32) {
	r, ok := tt.Find("sub")
	if !ok {
		panic(fmt.Sprintf("no subtitle: %q", tt.Texts()))
	}
	return r.X + 20, r.Y - 10
}

// iaCoverShelf is an ItemShelf of just it.
func iaCoverShelf(it services.Item, opener, play string, tweak func(*ItemsCtx)) iaSurface {
	return iaSurface{
		w: 400, h: 400, name: it.Name, opener: opener, play: play, at: iaUnderTitle, tweak: tweak,
		mk: func() (func(c *ui.Context, ctx ItemsCtx), func() int) {
			st := &CoverShelfState{}
			return func(c *ui.Context, ctx ItemsCtx) {
				ItemShelf(c, st, "Shelf", []services.Item{it}, ctx, ItemShelfOpts{First: true})
			}, func() int { return st.Row.Sel.Len() }
		},
	}
}

// iaWideRow is a WideRow of just it (the card has the subtitle "sub").
func iaWideRow(it services.Item, opener, play string, tweak func(*ItemsCtx)) iaSurface {
	return iaSurface{
		w: 600, h: 400, name: it.Name, opener: opener, play: play, at: iaUnderTitle, tweak: tweak,
		mk: func() (func(c *ui.Context, ctx ItemsCtx), func() int) {
			st := &WideRowState{}
			return func(c *ui.Context, ctx ItemsCtx) {
				WideRow(c, st, []services.Item{it}, ctx, WideRowOpts{Title: "Hero", First: true, Min: 240, Ratio: 2})
			}, nil
		},
	}
}

// iaCenter is the middle of the control called label.
func iaCenter(label string) func(tt *ui.Tester) (float32, float32) {
	return func(tt *ui.Tester) (float32, float32) {
		r, ok := tt.Find(label)
		if !ok {
			panic(fmt.Sprintf("no %q: %q", label, tt.Texts()))
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
}

// iaLinkList is a LinkList of just it.
func iaLinkList(it services.Item, opener string, tweak func(*ItemsCtx)) iaSurface {
	return iaSurface{
		w: 600, h: 300, name: it.Name, opener: opener, at: iaCenter(it.Name), tweak: tweak,
		mk: func() (func(c *ui.Context, ctx ItemsCtx), func() int) {
			st := &LinkListState{}
			return func(c *ui.Context, ctx ItemsCtx) {
				LinkList(c, st, []services.Item{it}, ctx, LinkListOpts{Title: "Explore", First: true})
			}, nil
		},
	}
}

// iaStationList is a StationList of just s; its player is the page's.
func iaStationList(s services.Station, opener, play string, tweak func(*ItemsCtx)) iaSurface {
	return iaSurface{
		w: 600, h: 300, name: s.Name, opener: opener, play: play, at: iaCenter(opener), tweak: tweak,
		mk: func() (func(c *ui.Context, ctx ItemsCtx), func() int) {
			st := &StationListState{}
			return func(c *ui.Context, ctx ItemsCtx) {
				StationList(c, st, []services.Station{s}, ctx, StationListOpts{
					Label: "Stations",
					OnPlay: func(s services.Station) {
						ctx.Players.PlayItem(services.Item{Kind: services.ItemStation, ID: s.ID, Station: &s})
					},
				})
			}, nil
		},
	}
}

// iaTrackShelf is a TrackShelf of three songs, the first of them driven.
func iaTrackShelf(tweak func(*ItemsCtx)) iaSurface {
	return iaSurface{
		w: 640, h: 500, name: "Song 1", opener: "Song 1", play: "Play Song 1", tweak: tweak,
		at: func(tt *ui.Tester) (float32, float32) {
			r, _ := tt.Find("Song 1")
			return r.X + 60, r.Y + 6
		},
		mk: func() (func(c *ui.Context, ctx ItemsCtx), func() int) {
			st := &TrackShelfState{}
			return func(c *ui.Context, ctx ItemsCtx) {
				TrackShelf(c, st, trackItems(3), ctx, TrackShelfOpts{Title: "Top Songs", First: true})
			}, func() int { return st.Sel.State().Len() }
		},
	}
}

// iaTrackRow is what each gesture did to a row of a TrackList, and to the
// artist and album cells of the row.
type iaTrackRow struct {
	click, double, enter, shiftEnter, space, ctrlSpace string
	shiftClick, ctrlClick, playButton, menu            string
	artist, artistDouble, artistShift, album           string
	albumShift                                         string
}

// iaTrackList drives a six-row TrackList (artist and album columns, the
// artist and the album open pages): the pointer gestures on row 2, the keys on
// row 0, the focused one.
func iaTrackList(t *testing.T) iaTrackRow {
	t.Helper()
	var albums []string
	fresh := func() (*collTrackApp, *ui.Tester) {
		albums = nil
		return newCollTrackApp(t, 6, 1000, 500, func(a *collTrackApp) {
			a.opts.OpenAlbum = func(id string) { albums = append(albums, id) }
			a.opts.Menus = iaMenus()
		})
	}
	fx := func(a *collTrackApp) string {
		var out []string
		if len(a.played) > 0 {
			out = append(out, fmt.Sprintf("played=%v", a.played))
		}
		if len(a.artists) > 0 {
			out = append(out, fmt.Sprintf("artists=%v", a.artists))
		}
		if len(albums) > 0 {
			out = append(out, fmt.Sprintf("albums=%v", albums))
		}
		if n := a.st.Sel.Len(); n > 0 {
			out = append(out, fmt.Sprintf("sel=%d", n))
		}
		if len(out) == 0 {
			return "nothing"
		}
		return strings.Join(out, " ")
	}
	gesture := func(do func(a *collTrackApp, tt *ui.Tester)) string {
		a, tt := fresh()
		tt.Frame()
		do(a, tt)
		return fx(a)
	}
	row := func(times int, mods ui.Modifiers, label string) string {
		return gesture(func(_ *collTrackApp, tt *ui.Tester) {
			x, y := collRowPos(t, tt, label)
			for range times {
				tt.ClickAtWith(mods, x, y)
			}
		})
	}
	cell := func(times int, mods ui.Modifiers, label string) string {
		return gesture(func(_ *collTrackApp, tt *ui.Tester) {
			x, y := collPos(t, tt, label)
			for range times {
				tt.ClickAtWith(mods, x, y)
			}
		})
	}
	key := func(mods ui.Modifiers, k ui.Key) string {
		return gesture(func(_ *collTrackApp, tt *ui.Tester) {
			if !iaTab(tt, "Song 0") {
				t.Fatal("Tab never reached the first row")
			}
			tt.Key(mods, k)
		})
	}
	var got iaTrackRow
	got.click = row(1, 0, "Song 2")
	got.double = row(2, 0, "Song 2")
	got.enter = key(0, ui.KeyEnter)
	got.shiftEnter = key(ui.Shift, ui.KeyEnter)
	got.space = key(0, ui.KeySpace)
	got.ctrlSpace = key(ui.Cmd, ui.KeySpace)
	got.shiftClick = row(1, ui.Shift, "Song 2")
	got.ctrlClick = row(1, ui.Cmd, "Song 2")
	got.playButton = gesture(func(_ *collTrackApp, tt *ui.Tester) {
		x, y := collRowPos(t, tt, "Song 2")
		tt.Move(x, y)
		tt.Frame()
		if err := tt.Click("Play Song 2"); err != nil {
			t.Fatal(err)
		}
	})
	got.menu = "none"
	{
		_, tt := fresh()
		tt.Frame()
		x, y := collRowPos(t, tt, "Song 2")
		tt.RightClickAt(x, y)
		tt.Frame()
		if tt.HasText("Do something") {
			got.menu = "opens"
		}
	}
	got.artist = cell(1, 0, "Artist 2")
	got.artistDouble = cell(2, 0, "Artist 2")
	got.artistShift = cell(1, ui.Shift, "Artist 2")
	got.album = cell(1, 0, "Album 2")
	got.albumShift = cell(1, ui.Shift, "Album 2")
	return got
}

// iaUnderlined moves the pointer onto the item of the surface and reports
// whether its title is underlined: its row of pixels (under the cover, left of
// the more button) differs from the same row with the pointer away.
func iaUnderlined(t *testing.T, s iaSurface) bool {
	t.Helper()
	build, _ := s.mk()
	a, tt := newItemsApp(t, s.w, s.h, nil, build)
	a.rec, a.tweak, a.menus = iaRec(), s.tweak, iaMenus()
	a.load(t, tt, 0)
	sub, ok := tt.Find("sub")
	if !ok {
		t.Fatalf("no subtitle: %q", tt.Texts())
	}
	tt.Move(1, 1)
	tt.Frame()
	title := func() []color.RGBA {
		var px []color.RGBA
		img := tt.Image()
		for y := int(sub.Y) - 20; y < int(sub.Y); y++ {
			for x := int(sub.X); x < int(sub.X)+50; x++ {
				px = append(px, img.RGBAAt(x, y))
			}
		}
		return px
	}
	rest := title()
	x, y := s.at(tt)
	tt.Move(x, y)
	tt.Frame()
	return !slices.Equal(rest, title())
}

// iaKind is an item of one kind, and what its opener is called on each surface
// (the subtitle of the cover and card is "sub").
type iaKind struct {
	name string
	item services.Item
	// cover is the opener of its cover tile; link the opener of its link tile.
	cover, link string
}

func iaKinds() []iaKind {
	sub := func(it services.Item) services.Item { it.Subtitle = "sub"; return it }
	album := sub(albumItem(1))
	album.Name = "Album One"
	upcoming := services.Album{ID: "up1", Name: "Upcoming One", Prerelease: true, ReleaseAt: time.Date(2026, 11, 19, 0, 0, 0, 0, time.Local).UnixMilli()}
	return []iaKind{
		{"song", sub(songItem("1", "Song One", "Artist", false)), "Song One", "Song One"},
		{"album", album, "Album One", "Album One"},
		{"playlist", sub(services.Item{Kind: services.ItemPlaylist, ID: "pl1", Name: "Playlist One", Playlist: &services.Playlist{ID: "pl1", Name: "Playlist One"}}), "Playlist One", "Playlist One"},
		{"artist", sub(services.Item{Kind: services.ItemArtist, ID: "ar1", Name: "Artist One", Artist: &services.Artist{ID: "ar1", Name: "Artist One"}}), "Artist One", "Artist One"},
		{"station", sub(stationItem(true, true)), "Local FM", "Local FM"},
		{"station without a page", sub(stationItem(false, true)), "Local FM Play", "Local FM Play"},
		{"unplayable station without a page", sub(stationItem(false, false)), "Local FM Opens in your browser", "Local FM Opens in your browser"},
		{"music video", sub(services.Item{Kind: services.ItemMusicVideo, ID: "v1", Name: "Video One", MusicVideo: &services.MusicVideo{ID: "v1", Name: "Video One"}}), "Video One", "Video One"},
		{"curator", sub(services.Item{Kind: services.ItemCurator, ID: "cu1", Name: "Curator One"}), "Curator One", "Curator One"},
		{"upcoming album", sub(services.Item{Kind: services.ItemAlbum, ID: "up1", Name: "Upcoming One", Album: &upcoming}), "Upcoming One, Out Nov 19", "Upcoming One"},
		{"web link", sub(services.Item{Kind: services.ItemLink, Name: "Web One", URL: "https://example.com/c"}), "Web One Opens in your browser", "Web One Opens in your browser"},
		{"room", sub(services.Item{Kind: services.ItemRoom, ID: "r1", Name: "Room One"}), "Room One", "Room One"},
	}
}

// iaNoRoute is a page whose router has no pages: nothing it links to is routed.
func iaNoRoute(ctx *ItemsCtx) { ctx.HasRoute = func(string) bool { return false } }

// iaSteps runs a sequence of gestures on one page, each step saying what the
// page's recorder and selection hold after it.
type iaStep struct {
	what string
	do   func()
	want string
}

func iaRun(t *testing.T, r func() *itemsRec, sel func() SelectionState, steps []iaStep) {
	t.Helper()
	for _, s := range steps {
		iaReset(r())
		s.do()
		got := iaFx(r())
		if keys := sel().Keys(); len(keys) > 0 {
			got += " keys=" + strings.Join(keys, ",")
		}
		if got != s.want {
			t.Errorf("%s: got %q, want %q", s.what, got, s.want)
		}
	}
}

func TestInteractionCoverTilesSelectOnlyWithModifiersAndKeys(t *testing.T) {
	st := &CoverShelfState{}
	a, tt := newItemsApp(t, 3*160+2*24+48, 500, nil, func(c *ui.Context, ctx ItemsCtx) {
		ItemShelf(c, st, "Albums", albumItems(3), ctx, ItemShelfOpts{First: true})
	})
	a.rec, a.menus = iaRec(), iaMenus()
	a.load(t, tt, 0)
	at := func(sub string) (float32, float32) {
		r, _ := tt.Find(sub)
		return r.X + 20, r.Y - 10
	}
	x1, y1 := at("Artist 1")
	x2, y2 := at("Artist 2")
	x3, y3 := at("Artist 3")
	iaRun(t, func() *itemsRec { return a.rec }, st.Row.Sel.State, []iaStep{
		// DEPARTS: nothing shows before a modifier-click that a tile can be selected (no checkbox, no hint); a plain click opens it.
		{"Ctrl+click tile 2", func() { tt.ClickAtWith(ui.Cmd, x2, y2) }, "nothing keys=1:album:2"},
		{"Shift+click tile 3 (the range from the anchor)", func() { tt.ClickAtWith(ui.Shift, x3, y3) }, "nothing keys=1:album:2,2:album:3"},
		{"click tile 1 (opens it, the selection stays)", func() { tt.ClickAt(x1, y1) }, "nav=/album/1 opened=album:1 keys=1:album:2,2:album:3"},
		{"Tab to tile 1", func() { iaTab(tt, "Album 1") }, "nothing keys=1:album:2,2:album:3"},
		{"Enter opens the focused tile, not the selection", func() { tt.Key(0, ui.KeyEnter) }, "nav=/album/1 opened=album:1 keys=1:album:2,2:album:3"},
		{"Ctrl+A", func() { tt.Key(ui.Cmd, ui.KeyA) }, "nothing keys=0:album:1,1:album:2,2:album:3"},
		{"Play disc of tile 2 plays that tile alone", func() { _ = tt.Click("Play Album 2") }, "played=album:2 keys=0:album:1,1:album:2,2:album:3"},
		{"click tile 1 with all three selected", func() { tt.ClickAt(x1, y1) }, "nav=/album/1 opened=album:1 keys=0:album:1,1:album:2,2:album:3"},
		{"Escape", func() { tt.Key(0, ui.KeyEscape) }, "nothing"},
		{"Tab to tile 2", func() { iaTab(tt, "Album 2") }, "nothing"},
		{"Ctrl+Space toggles the focused tile", func() { tt.Key(ui.Cmd, ui.KeySpace) }, "nothing keys=1:album:2"},
		{"Space does nothing on a link tile", func() { tt.Key(0, ui.KeySpace) }, "nothing keys=1:album:2"},
		{"Shift+Right extends the selection", func() { tt.Key(ui.Shift, ui.KeyRight) }, "nothing keys=1:album:2,2:album:3"},
		{"Shift+Enter does nothing", func() { tt.Key(ui.Shift, ui.KeyEnter) }, "nothing keys=1:album:2,2:album:3"},
		{"a press outside the list clears it", func() { tt.ClickAt(5, 450) }, "nothing"},
	})
}

// A tile's title underlines under the pointer, and nothing else tells a tile
// that opens from one that does nothing.
func TestInteractionCoverTitlesUnderlineUnderThePointerWhetherOrNotTheTileOpens(t *testing.T) {
	kinds := iaKinds()
	for _, k := range kinds {
		if !iaUnderlined(t, iaCoverShelf(k.item, k.cover, "", nil)) {
			t.Errorf("%s: a tile's title is not underlined under the pointer", k.name)
		}
	}
	song := kinds[0]
	// DEPARTS: a tile whose page is not routed has no Open and is a plain group, yet its title underlines under the pointer like a link.
	if !iaUnderlined(t, iaCoverShelf(song.item, song.cover, "", iaNoRoute)) {
		t.Errorf("a tile with no route is not underlined")
	}
}

// ItemGrid builds the tiles of ItemShelf: the same gestures do the same.
func TestInteractionItemGridTilesDoWhatShelfTilesDo(t *testing.T) {
	items := []services.Item{albumItem(1), iaKinds()[5].item}
	items[1].Subtitle = "sub2"
	var st ItemGridState
	a, tt := newItemsApp(t, 500, 500, nil, func(c *ui.Context, ctx ItemsCtx) {
		ItemGrid(c, &st, len(items), func(i int) *services.Item { return &items[i] }, ctx, ItemGridOpts{Label: "Grid"})
	})
	a.rec, a.menus = iaRec(), iaMenus()
	a.load(t, tt, 0)
	r1, _ := tt.Find("Artist 1")
	r2, _ := tt.Find("sub2")
	iaRun(t, func() *itemsRec { return a.rec }, st.Sel.State, []iaStep{
		{"click an album", func() { tt.ClickAt(r1.X+20, r1.Y-10) }, "nav=/album/1 opened=album:1"},
		{"Ctrl+click an album", func() { tt.ClickAtWith(ui.Cmd, r1.X+20, r1.Y-10) }, "nothing keys=0:album:1"},
		{"its Play disc", func() { _ = tt.Click("Play Album 1") }, "played=album:1 keys=0:album:1"},
		{"click a station without a page", func() { tt.ClickAt(r2.X+20, r2.Y-10) }, "opened=station:ra.1 played=station:ra.1 keys=0:album:1"},
		{"Enter on it", func() { iaTab(tt, "Local FM Play"); tt.Key(0, ui.KeyEnter) }, "opened=station:ra.1 played=station:ra.1 keys=0:album:1"},
	})
}

// CoverShelf is the tile without the item behind it: what a tile does follows
// from its Open, Play and PlaysOnOpen, and what the opener is on the
// accessibility tree from the same.
func TestInteractionCoverShelfTilesFollowTheirOpenPlayAndPlaysOnOpen(t *testing.T) {
	var opened, played []string
	st := &CoverShelfState{}
	album := AlbumItem(services.Album{ID: "1"})
	tiles := []CoverItem{
		{
			Key: "a", Title: "Openable", Subtitle: "s1", Item: &album, Open: func() { opened = append(opened, "a") },
			Play: &CoverPlay{Label: "Openable", Run: func() { played = append(played, "a") }},
		},
		{Key: "b", Title: "Closed", Subtitle: "s2", Item: &album},
		{Key: "c", Title: "Plays", Subtitle: "s3", PlaysOnOpen: true, Open: func() { played = append(played, "c") }},
		{Key: "d", Title: "No item", Subtitle: "s4", Open: func() { opened = append(opened, "d") }},
	}
	mt := newMainThread()
	art := newArtLoader(&fakeArt{}, mt.post, 2)
	t.Cleanup(art.Close)
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Fill().Padding(24).Children(func() {
			CoverShelf(c, st, "Shelf", tiles, CoverShelfOpts{CoverOpts: CoverOpts{Label: "Shelf", Art: art, Menus: iaMenus()}, First: true})
		})
	}, 900, 400)
	tt.Frame()
	for _, c := range []struct{ opener, role string }{
		{"Openable", "link"}, {"Closed", "group"}, {"Plays Play", "button"}, {"No item", "link"},
	} {
		if got := iaRole(tt, c.opener); got != c.role {
			t.Errorf("%s: role %q, want %q", c.opener, got, c.role)
		}
	}
	got := func() string {
		s := fmt.Sprintf("opened=%v played=%v sel=%d", opened, played, st.Row.Sel.Len())
		opened, played = nil, nil
		return s
	}
	click := func(sub string, mods ui.Modifiers) string {
		r, _ := tt.Find(sub)
		tt.ClickAtWith(mods, r.X+20, r.Y-10)
		return got()
	}
	key := func(opener string, k ui.Key) string {
		if !iaTab(tt, opener) {
			return "not focusable"
		}
		tt.Key(0, k)
		return got()
	}
	for _, c := range []struct{ what, got, want string }{
		{"click a tile with Open", click("s1", 0), "opened=[a] played=[] sel=0"},
		{"Enter on it", key("Openable", ui.KeyEnter), "opened=[a] played=[] sel=0"},
		{"its Play control", func() string { _ = tt.Click("Play Openable"); return got() }(), "opened=[] played=[a] sel=0"},
		// DEPARTS: a tile with an item but no Open takes the focus and underlines on hover like a link, and does nothing on click or Enter.
		{"click a tile without Open", click("s2", 0), "opened=[] played=[] sel=0"},
		{"Enter on it", key("Closed", ui.KeyEnter), "opened=[] played=[] sel=0"},
		{"Ctrl+click it", click("s2", ui.Cmd), "opened=[] played=[] sel=1"},
		{"click a tile that plays on open", click("s3", 0), "opened=[] played=[c] sel=1"},
		{"Enter on it", key("Plays Play", ui.KeyEnter), "opened=[] played=[c] sel=1"},
		// DEPARTS: a modifier-click on a tile without an item is swallowed: it neither selects nor opens.
		{"Ctrl+click a tile without an item", func() string { st.Row.Sel.Reset(); return click("s4", ui.Cmd) }(), "opened=[] played=[] sel=0"},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.what, c.got, c.want)
		}
	}
}

// A song in a TrackShelf: its artist is a link, that opens the artist and
// does not play the song.
func TestInteractionTrackShelfArtistLinkOpensTheArtistAndTheRowStaysQuiet(t *testing.T) {
	a, tt := newItemsApp(t, 640, 500, nil, func(c *ui.Context, ctx ItemsCtx) {
		TrackShelf(c, &TrackShelfState{}, trackItems(3), ctx, TrackShelfOpts{Title: "Top Songs", First: true})
	})
	a.rec, a.menus = iaRec(), iaMenus()
	a.load(t, tt, 0)
	r, _ := tt.Find("Artist 1")
	x, y := r.X+r.W/2, r.Y+r.H/2
	for _, c := range []struct {
		what string
		do   func()
		want string
	}{
		{"click the artist", func() { tt.ClickAt(x, y) }, "nav=/artist/ar1"},
		{"double-click the artist", func() { tt.ClickAt(x, y); tt.ClickAt(x, y) }, "nav=/artist/ar1"},
		{"Shift+click the artist", func() { tt.ClickAtWith(ui.Shift, x, y) }, "nav=/artist/ar1"},
	} {
		iaReset(a.rec)
		c.do()
		if got := iaFx(a.rec); got != c.want {
			t.Errorf("%s: got %q, want %q", c.what, got, c.want)
		}
	}
}

// iaCoverWant is every kind as a cover tile of an ItemShelf (the real app's players).
var iaCoverWant = map[string]iaRow{
	"song": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/song/1 opened=song:1",
		double:     "nav=/song/1 opened=song:1",
		enter:      "nav=/song/1 opened=song:1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "played=song:1",
	},
	"album": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/album/1 opened=album:1",
		double:     "nav=/album/1 opened=album:1",
		enter:      "nav=/album/1 opened=album:1",
		shiftEnter: "nothing",
		space:      "nothing",
		// DEPARTS: a Shift/Ctrl+click selects the tile with no visible cue before it (no checkbox, no hint); the app-wide selection model, but nothing on the tile promises it.
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "played=album:1",
	},
	"playlist": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/playlist/pl1 opened=playlist:pl1",
		double:     "nav=/playlist/pl1 opened=playlist:pl1",
		enter:      "nav=/playlist/pl1 opened=playlist:pl1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "played=playlist:pl1",
	},
	"artist": {
		role: "link",
		// DEPARTS: no Play control for this kind while song, album, playlist, station and music video tiles have one: no player is registered for it (features/discover/players.go registerPlayers), so ItemPlayers.CanPlay is false; fix: register an artist/curator player or leave it out of the rule.
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "nav=/artist/ar1 opened=artist:ar1",
		double:     "nav=/artist/ar1 opened=artist:ar1",
		enter:      "nav=/artist/ar1 opened=artist:ar1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "no control",
	},
	"station": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/station/ra.1 opened=station:ra.1",
		double:     "nav=/station/ra.1 opened=station:ra.1",
		enter:      "nav=/station/ra.1 opened=station:ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "played=station:ra.1",
	},
	"station without a page": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "opened=station:ra.1 played=station:ra.1",
		double:     "opened=station:ra.1 played=station:ra.1",
		enter:      "opened=station:ra.1 played=station:ra.1",
		shiftEnter: "nothing",
		space:      "opened=station:ra.1 played=station:ra.1",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "no control",
	},
	"unplayable station without a page": {
		role:       "link",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		double:     "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		enter:      "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "no control",
	},
	"music video": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/music-video/v1 opened=music-video:v1",
		double:     "nav=/music-video/v1 opened=music-video:v1",
		enter:      "nav=/music-video/v1 opened=music-video:v1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "videos=v1",
	},
	"curator": {
		role: "link",
		// DEPARTS: no Play control for this kind while song, album, playlist, station and music video tiles have one: no player is registered for it (features/discover/players.go registerPlayers), so ItemPlayers.CanPlay is false; fix: register an artist/curator player or leave it out of the rule.
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/curator/cu1 opened=curator:cu1",
		double:     "nav=/curator/cu1 opened=curator:cu1",
		enter:      "nav=/curator/cu1 opened=curator:cu1",
		shiftEnter: "nothing",
		space:      "nothing",
		// DEPARTS: a Shift/Ctrl+click on a tile with no Item (curator, room, web link) is swallowed: it neither selects nor opens, while it selects an album tile and opens a WideRow card or LinkList tile (collections_cover.go:323-331: only a tile with an Item takes a modified click).
		shiftClick: "nothing",
		ctrlClick:  "nothing",
		playDisc:   "no control",
	},
	"upcoming album": {
		role:       "link",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "nav=/album/up1 opened=album:up1",
		double:     "nav=/album/up1 opened=album:up1",
		enter:      "nav=/album/up1 opened=album:up1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "no control",
	},
	"web link": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "opened=link: urls=https://example.com/c",
		double:     "opened=link: urls=https://example.com/c",
		enter:      "opened=link: urls=https://example.com/c",
		shiftEnter: "nothing",
		space:      "nothing",
		// DEPARTS: a Shift/Ctrl+click on a tile with no Item (curator, room, web link) is swallowed: it neither selects nor opens, while it selects an album tile and opens a WideRow card or LinkList tile (collections_cover.go:323-331: only a tile with an Item takes a modified click).
		shiftClick: "nothing",
		ctrlClick:  "nothing",
		playDisc:   "no control",
	},
	"room": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/room/r1 opened=room:r1",
		double:     "nav=/room/r1 opened=room:r1",
		enter:      "nav=/room/r1 opened=room:r1",
		shiftEnter: "nothing",
		space:      "nothing",
		// DEPARTS: a Shift/Ctrl+click on a tile with no Item (curator, room, web link) is swallowed: it neither selects nor opens, while it selects an album tile and opens a WideRow card or LinkList tile (collections_cover.go:323-331: only a tile with an Item takes a modified click).
		shiftClick: "nothing",
		ctrlClick:  "nothing",
		playDisc:   "no control",
	},
	"song without a route": {
		role: "group",
		play: "disc",
		more: "button",
		menu: "opens",
		// DEPARTS: with no route for its page the tile still looks like a link (title underlines on hover, focusable) but a click and Enter do nothing and it is a plain group; its Play disc stays (collections_items_cover.go:118-123: no Open when HasRoute says no).
		click:      "nothing",
		double:     "nothing",
		enter:      "nothing",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "played=song:1",
	},
}

// iaWideWant is every kind as a WideRow card.
var iaWideWant = map[string]iaRow{
	"song": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/song/1 opened=song:1",
		double:     "nav=/song/1 opened=song:1",
		enter:      "nav=/song/1 opened=song:1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/song/1 opened=song:1",
		ctrlClick:  "nav=/song/1 opened=song:1",
		playDisc:   "played=song:1",
	},
	"album": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/album/1 opened=album:1",
		double:     "nav=/album/1 opened=album:1",
		enter:      "nav=/album/1 opened=album:1",
		shiftEnter: "nothing",
		space:      "nothing",
		// DEPARTS: a Shift/Ctrl+click opens the card (modifiers are ignored); on a cover tile it selects instead (collections_items_widerow.go card.Clicked).
		shiftClick: "nav=/album/1 opened=album:1",
		ctrlClick:  "nav=/album/1 opened=album:1",
		playDisc:   "played=album:1",
	},
	"playlist": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/playlist/pl1 opened=playlist:pl1",
		double:     "nav=/playlist/pl1 opened=playlist:pl1",
		enter:      "nav=/playlist/pl1 opened=playlist:pl1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/playlist/pl1 opened=playlist:pl1",
		ctrlClick:  "nav=/playlist/pl1 opened=playlist:pl1",
		playDisc:   "played=playlist:pl1",
	},
	"artist": {
		role: "link",
		// DEPARTS: no Play control for this kind while song, album, playlist, station and music video tiles have one: no player is registered for it (features/discover/players.go registerPlayers), so ItemPlayers.CanPlay is false; fix: register an artist/curator player or leave it out of the rule.
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "nav=/artist/ar1 opened=artist:ar1",
		double:     "nav=/artist/ar1 opened=artist:ar1",
		enter:      "nav=/artist/ar1 opened=artist:ar1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/artist/ar1 opened=artist:ar1",
		ctrlClick:  "nav=/artist/ar1 opened=artist:ar1",
		playDisc:   "no control",
	},
	"station": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/station/ra.1 opened=station:ra.1",
		double:     "nav=/station/ra.1 opened=station:ra.1",
		enter:      "nav=/station/ra.1 opened=station:ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/station/ra.1 opened=station:ra.1",
		ctrlClick:  "nav=/station/ra.1 opened=station:ra.1",
		playDisc:   "played=station:ra.1",
	},
	"station without a page": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "opened=station:ra.1 played=station:ra.1",
		double:     "opened=station:ra.1 played=station:ra.1",
		enter:      "opened=station:ra.1 played=station:ra.1",
		shiftEnter: "nothing",
		space:      "opened=station:ra.1 played=station:ra.1",
		shiftClick: "opened=station:ra.1 played=station:ra.1",
		ctrlClick:  "opened=station:ra.1 played=station:ra.1",
		playDisc:   "no control",
	},
	"unplayable station without a page": {
		role:       "link",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		double:     "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		enter:      "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		ctrlClick:  "opened=station:ra.1 urls=https://music.apple.com/gb/station/local-fm/ra.1",
		playDisc:   "no control",
	},
	"music video": {
		role:       "link",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "nav=/music-video/v1 opened=music-video:v1",
		double:     "nav=/music-video/v1 opened=music-video:v1",
		enter:      "nav=/music-video/v1 opened=music-video:v1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/music-video/v1 opened=music-video:v1",
		ctrlClick:  "nav=/music-video/v1 opened=music-video:v1",
		playDisc:   "videos=v1",
	},
	"curator": {
		role: "link",
		// DEPARTS: no Play control for this kind while song, album, playlist, station and music video tiles have one: no player is registered for it (features/discover/players.go registerPlayers), so ItemPlayers.CanPlay is false; fix: register an artist/curator player or leave it out of the rule.
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/curator/cu1 opened=curator:cu1",
		double:     "nav=/curator/cu1 opened=curator:cu1",
		enter:      "nav=/curator/cu1 opened=curator:cu1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/curator/cu1 opened=curator:cu1",
		ctrlClick:  "nav=/curator/cu1 opened=curator:cu1",
		playDisc:   "no control",
	},
	"upcoming album": {
		role:       "link",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "nav=/album/up1 opened=album:up1",
		double:     "nav=/album/up1 opened=album:up1",
		enter:      "nav=/album/up1 opened=album:up1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/album/up1 opened=album:up1",
		ctrlClick:  "nav=/album/up1 opened=album:up1",
		playDisc:   "no control",
	},
	"web link": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "opened=link: urls=https://example.com/c",
		double:     "opened=link: urls=https://example.com/c",
		enter:      "opened=link: urls=https://example.com/c",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "opened=link: urls=https://example.com/c",
		ctrlClick:  "opened=link: urls=https://example.com/c",
		playDisc:   "no control",
	},
	"room": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/room/r1 opened=room:r1",
		double:     "nav=/room/r1 opened=room:r1",
		enter:      "nav=/room/r1 opened=room:r1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/room/r1 opened=room:r1",
		ctrlClick:  "nav=/room/r1 opened=room:r1",
		playDisc:   "no control",
	},
	"song without a route": {
		role: "link",
		play: "disc",
		more: "button",
		menu: "opens",
		// DEPARTS: WideRow ignores ItemsCtx.HasRoute: a card whose page is not routed still navigates to it (the start page, in the app) where the cover tile builds without Open (collections_items_widerow.go:235-238 card.Clicked, ctx.nav(link)).
		click:      "nav=/song/1 opened=song:1",
		double:     "nav=/song/1 opened=song:1",
		enter:      "nav=/song/1 opened=song:1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/song/1 opened=song:1",
		ctrlClick:  "nav=/song/1 opened=song:1",
		playDisc:   "played=song:1",
	},
}

// iaLinkWant is every kind as a LinkList tile.
var iaLinkWant = map[string]iaRow{
	"song": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/song/1",
		double:     "nav=/song/1",
		enter:      "nav=/song/1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/song/1",
		ctrlClick:  "nav=/song/1",
		playDisc:   "no control",
	},
	"album": {
		role: "link",
		play: "none",
		more: "none",
		menu: "none",
		// DEPARTS: OnOpen does not run, so search does not remember the item (cover tiles and WideRow cards run it; collections_items_linklist.go:104-109 ctx.nav without ctx.opened); same for every kind in LinkList and StationList.
		click:      "nav=/album/1",
		double:     "nav=/album/1",
		enter:      "nav=/album/1",
		shiftEnter: "nothing",
		space:      "nothing",
		// DEPARTS: a Shift/Ctrl+click opens the tile (modifiers are ignored); a cover tile selects.
		shiftClick: "nav=/album/1",
		ctrlClick:  "nav=/album/1",
		playDisc:   "no control",
	},
	"playlist": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/playlist/pl1",
		double:     "nav=/playlist/pl1",
		enter:      "nav=/playlist/pl1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/playlist/pl1",
		ctrlClick:  "nav=/playlist/pl1",
		playDisc:   "no control",
	},
	"artist": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/artist/ar1",
		double:     "nav=/artist/ar1",
		enter:      "nav=/artist/ar1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/artist/ar1",
		ctrlClick:  "nav=/artist/ar1",
		playDisc:   "no control",
	},
	"station": {
		role: "link",
		// DEPARTS: a station with a page has no Play control in LinkList while its cover tile, WideRow card and StationList row have one.
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/station/ra.1",
		double:     "nav=/station/ra.1",
		enter:      "nav=/station/ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/station/ra.1",
		ctrlClick:  "nav=/station/ra.1",
		playDisc:   "no control",
	},
	"station without a page": {
		role:       "button",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "played=station:ra.1",
		double:     "played=station:ra.1",
		enter:      "played=station:ra.1",
		shiftEnter: "nothing",
		space:      "played=station:ra.1",
		shiftClick: "played=station:ra.1",
		ctrlClick:  "played=station:ra.1",
		playDisc:   "no control",
	},
	"unplayable station without a page": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "urls=https://music.apple.com/gb/station/local-fm/ra.1",
		double:     "urls=https://music.apple.com/gb/station/local-fm/ra.1",
		enter:      "not focusable",
		shiftEnter: "not focusable",
		space:      "not focusable",
		shiftClick: "urls=https://music.apple.com/gb/station/local-fm/ra.1",
		ctrlClick:  "urls=https://music.apple.com/gb/station/local-fm/ra.1",
		playDisc:   "no control",
	},
	"music video": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/music-video/v1",
		double:     "nav=/music-video/v1",
		enter:      "nav=/music-video/v1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/music-video/v1",
		ctrlClick:  "nav=/music-video/v1",
		playDisc:   "no control",
	},
	"curator": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/curator/cu1",
		double:     "nav=/curator/cu1",
		enter:      "nav=/curator/cu1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/curator/cu1",
		ctrlClick:  "nav=/curator/cu1",
		playDisc:   "no control",
	},
	"upcoming album": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/album/up1",
		double:     "nav=/album/up1",
		enter:      "nav=/album/up1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/album/up1",
		ctrlClick:  "nav=/album/up1",
		playDisc:   "no control",
	},
	"web link": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "urls=https://example.com/c",
		double:     "urls=https://example.com/c",
		enter:      "not focusable",
		shiftEnter: "not focusable",
		space:      "not focusable",
		shiftClick: "urls=https://example.com/c",
		ctrlClick:  "urls=https://example.com/c",
		playDisc:   "no control",
	},
	"room": {
		role:       "link",
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/room/r1",
		double:     "nav=/room/r1",
		enter:      "nav=/room/r1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/room/r1",
		ctrlClick:  "nav=/room/r1",
		playDisc:   "no control",
	},
	"room without a route": {
		role: "link",
		play: "none",
		more: "none",
		menu: "none",
		// DEPARTS: LinkList ignores ItemsCtx.HasRoute: a link to a page that is not routed navigates to it (the start page, in the app); cover tiles build without Open (collections_items_linklist.go:104-109).
		click:      "nav=/room/r1",
		double:     "nav=/room/r1",
		enter:      "nav=/room/r1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/room/r1",
		ctrlClick:  "nav=/room/r1",
		playDisc:   "no control",
	},
}

// iaStationWant is a station as a StationList row.
var iaStationWant = map[string]iaRow{
	"with a page": {
		role: "link",
		play: "disc",
		// DEPARTS: a station with a page has no more button and no menu here, where its cover tile has both (collections_items_stations.go:157: Menus.Trigger only for stations without a page).
		more: "none",
		menu: "none",
		// DEPARTS: OnOpen does not run (search does not remember the station); cover tiles and WideRow cards run it (collections_items_stations.go:164 ctx.nav only).
		click:      "nav=/station/ra.1",
		double:     "nav=/station/ra.1",
		enter:      "nav=/station/ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/station/ra.1",
		ctrlClick:  "nav=/station/ra.1",
		playDisc:   "played=station:ra.1",
	},
	"without a page": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "played=station:ra.1",
		double:     "played=station:ra.1",
		enter:      "played=station:ra.1",
		shiftEnter: "nothing",
		space:      "played=station:ra.1",
		shiftClick: "played=station:ra.1",
		ctrlClick:  "played=station:ra.1",
		playDisc:   "no control",
	},
	"without a page, playing": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "toggles=1",
		double:     "toggles=1",
		enter:      "toggles=1",
		shiftEnter: "nothing",
		space:      "toggles=1",
		shiftClick: "toggles=1",
		ctrlClick:  "toggles=1",
		playDisc:   "no control",
	},
	"without a page, paused": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "toggles=1",
		double:     "toggles=1",
		enter:      "toggles=1",
		shiftEnter: "nothing",
		space:      "toggles=1",
		shiftClick: "toggles=1",
		ctrlClick:  "toggles=1",
		playDisc:   "no control",
	},
	"with a page, playing": {
		role: "link",
		// DEPARTS: the Play button gives way to the equaliser, so a playing station with a page cannot be paused from its row (only the page-less one toggles); the cover tile has the Play disc throughout.
		play:       "none",
		more:       "none",
		menu:       "none",
		click:      "nav=/station/ra.1",
		double:     "nav=/station/ra.1",
		enter:      "nav=/station/ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/station/ra.1",
		ctrlClick:  "nav=/station/ra.1",
		playDisc:   "no control",
	},
	"unplayable with a page": {
		role:       "link",
		play:       "disc",
		more:       "none",
		menu:       "none",
		click:      "nav=/station/ra.1",
		double:     "nav=/station/ra.1",
		enter:      "nav=/station/ra.1",
		shiftEnter: "nothing",
		space:      "nothing",
		shiftClick: "nav=/station/ra.1",
		ctrlClick:  "nav=/station/ra.1",
		playDisc:   "nothing",
	},
	"unplayable without a page": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "none",
		click:      "nothing",
		double:     "nothing",
		enter:      "not focusable",
		shiftEnter: "not focusable",
		space:      "not focusable",
		shiftClick: "nothing",
		ctrlClick:  "nothing",
		playDisc:   "no control",
	},
}

// iaTrackShelfWant is a song as a TrackShelf row.
var iaTrackShelfWant = map[string]iaRow{
	"song": {
		role:       "list item",
		play:       "disc",
		more:       "button",
		menu:       "opens",
		click:      "sel=1",
		double:     "songs=Top Songs@Song 1/Song 1..Song 3 sel=1",
		enter:      "songs=Top Songs@Song 1/Song 1..Song 3",
		shiftEnter: "nothing",
		space:      "sel=1",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "songs=Top Songs@Song 1/Song 1..Song 3",
	},
}

// iaCoverPlayingWant is the tile of the station that plays.
var iaCoverPlayingWant = map[string]iaRow{
	"station without a page, playing": {
		role:       "button",
		play:       "none",
		more:       "button",
		menu:       "opens",
		click:      "opened=station:ra.1 toggles=1",
		double:     "opened=station:ra.1 toggles=1",
		enter:      "opened=station:ra.1 toggles=1",
		shiftEnter: "nothing",
		space:      "opened=station:ra.1 toggles=1",
		shiftClick: "sel=1",
		ctrlClick:  "sel=1",
		playDisc:   "no control",
	},
}

// iaTrackListWant is a row of a TrackList (the kit's, as the album, playlist,
// library and search pages use it) and its artist and album cells.
var iaTrackListWant = iaTrackRow{
	click:      "sel=1",
	double:     "played=[2] sel=1",
	enter:      "played=[0]",
	shiftEnter: "nothing",
	// DEPARTS: Space selects the row (a keyboard click, collections_tracklist.go:862 src.Clicked) while Enter plays it and Ctrl+Space is the documented toggle; nothing hints at either.
	space:      "sel=1",
	ctrlSpace:  "sel=1",
	shiftClick: "sel=1",
	ctrlClick:  "sel=1",
	playButton: "played=[2]",
	menu:       "opens",
	artist:     "artists=[ar2]",
	// A double-click on the artist link opens the artist once and does not play the row.
	artistDouble: "artists=[ar2]",
	artistShift:  "sel=1",
	album:        "albums=[al2]",
	albumShift:   "sel=1",
}

func TestInteractionCoverTiles(t *testing.T) {
	for _, k := range iaKinds() {
		t.Run(k.name, func(t *testing.T) {
			iaDiff(t, k.name, iaDrive(t, iaCoverShelf(k.item, k.cover, "Play "+k.item.Name, nil)), iaCoverWant[k.name])
		})
	}
	song := iaKinds()[0]
	t.Run("song without a route", func(t *testing.T) {
		iaDiff(t, "song without a route", iaDrive(t, iaCoverShelf(song.item, song.cover, "Play "+song.item.Name, iaNoRoute)), iaCoverWant["song without a route"])
	})
}

func TestInteractionCoverTileOfThePlayingStation(t *testing.T) {
	k := iaKinds()[5]
	playing := func(ctx *ItemsCtx) { ctx.Current, ctx.Playing = &services.QueueItem{ID: "ra.1"}, true }
	iaDiff(t, "playing", iaDrive(t, iaCoverShelf(k.item, k.item.Name+" Pause", "Play "+k.item.Name, playing)), iaCoverPlayingWant["station without a page, playing"])
}

// iaWidePlayingWant is the wide card of the station that plays: its one
// control reads "Pause" and every press pauses it.
var iaWidePlayingWant = iaRow{
	role:       "button",
	play:       "none",
	more:       "button",
	menu:       "opens",
	click:      "opened=station:ra.1 toggles=1",
	double:     "opened=station:ra.1 toggles=1",
	enter:      "opened=station:ra.1 toggles=1",
	shiftEnter: "nothing",
	space:      "opened=station:ra.1 toggles=1",
	shiftClick: "opened=station:ra.1 toggles=1",
	ctrlClick:  "opened=station:ra.1 toggles=1",
	playDisc:   "no control",
}

// iaLinkPlayingWant is the link tile of the station that plays.
var iaLinkPlayingWant = iaRow{
	role:       "button",
	play:       "none",
	more:       "none",
	menu:       "none",
	click:      "toggles=1",
	double:     "toggles=1",
	enter:      "toggles=1",
	shiftEnter: "nothing",
	space:      "toggles=1",
	shiftClick: "toggles=1",
	ctrlClick:  "toggles=1",
	playDisc:   "no control",
}

func TestInteractionWideRowAndLinkTileOfThePlayingStation(t *testing.T) {
	k := iaKinds()[5]
	playing := func(ctx *ItemsCtx) { ctx.Current, ctx.Playing = &services.QueueItem{ID: "ra.1"}, true }
	iaDiff(t, "wide card", iaDrive(t, iaWideRow(k.item, k.item.Name+" Pause", "Play "+k.item.Name, playing)), iaWidePlayingWant)
	iaDiff(t, "link tile", iaDrive(t, iaLinkList(k.item, k.item.Name+" Pause", playing)), iaLinkPlayingWant)
}

// A station that is on but paused offers Play again, and pressing it resumes.
func TestStationControlReadsPlayWhilePaused(t *testing.T) {
	k := iaKinds()[5]
	paused := func(ctx *ItemsCtx) { ctx.Current, ctx.Playing = &services.QueueItem{ID: "ra.1"}, false }
	iaDiff(t, "paused link tile", iaDrive(t, iaLinkList(k.item, k.item.Name+" Play", paused)), iaLinkPlayingWant)
}

func TestInteractionWideRowCards(t *testing.T) {
	for _, k := range iaKinds() {
		t.Run(k.name, func(t *testing.T) {
			opener := k.item.Name
			if strings.HasSuffix(k.link, "Opens in your browser") || k.name == "station without a page" {
				opener = k.link // a web-only or page-less card is named as its link tile is
			}
			iaDiff(t, k.name, iaDrive(t, iaWideRow(k.item, opener, "Play "+k.item.Name, nil)), iaWideWant[k.name])
		})
	}
	song := iaKinds()[0]
	t.Run("song without a route", func(t *testing.T) {
		iaDiff(t, "song without a route", iaDrive(t, iaWideRow(song.item, song.item.Name, "Play "+song.item.Name, iaNoRoute)), iaWideWant["song without a route"])
	})
}

func TestInteractionLinkListTiles(t *testing.T) {
	for _, k := range iaKinds() {
		t.Run(k.name, func(t *testing.T) {
			iaDiff(t, k.name, iaDrive(t, iaLinkList(k.item, k.link, nil)), iaLinkWant[k.name])
		})
	}
	room := iaKinds()[11]
	t.Run("room without a route", func(t *testing.T) {
		iaDiff(t, "room without a route", iaDrive(t, iaLinkList(room.item, room.link, iaNoRoute)), iaLinkWant["room without a route"])
	})
}

func TestInteractionStationListRows(t *testing.T) {
	station := func(page, playable bool) services.Station {
		return services.Station{
			ID: "ra.1", Name: "Local FM", Provider: "TuneIn", Kind: services.StationBroadcast, HasPage: page, Playable: playable,
			Art: services.Art{Small: "/art/kra.1/96x96.jpg"},
		}
	}
	current := func(playing bool) func(*ItemsCtx) {
		return func(ctx *ItemsCtx) { ctx.Current, ctx.Playing = &services.QueueItem{ID: "ra.1"}, playing }
	}
	for _, c := range []struct {
		name         string
		s            services.Station
		opener, play string
		tweak        func(*ItemsCtx)
	}{
		{"with a page", station(true, true), "Local FM", "Play Local FM", nil},
		{"without a page", station(false, true), "Local FM Play", "Play Local FM", nil},
		{"without a page, playing", station(false, true), "Local FM Pause", "Play Local FM", current(true)},
		{"without a page, paused", station(false, true), "Local FM Play", "Play Local FM", current(false)},
		{"with a page, playing", station(true, true), "Local FM", "Play Local FM", current(true)},
		{"unplayable with a page", station(true, false), "Local FM", "Play Local FM", nil},
		{"unplayable without a page", station(false, false), "Local FM Play", "Play Local FM", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			iaDiff(t, c.name, iaDrive(t, iaStationList(c.s, c.opener, c.play, c.tweak)), iaStationWant[c.name])
		})
	}
}

func TestInteractionTrackShelfRows(t *testing.T) {
	iaDiff(t, "song", iaDrive(t, iaTrackShelf(nil)), iaTrackShelfWant["song"])
}

func TestInteractionTrackListRows(t *testing.T) {
	got, want := iaTrackList(t), iaTrackListWant
	for _, c := range []struct{ what, got, want string }{
		{"click", got.click, want.click},
		{"double-click", got.double, want.double},
		{"Enter", got.enter, want.enter},
		{"Shift+Enter", got.shiftEnter, want.shiftEnter},
		{"Space", got.space, want.space},
		{"Ctrl+Space", got.ctrlSpace, want.ctrlSpace},
		{"Shift+click", got.shiftClick, want.shiftClick},
		{"Ctrl+click", got.ctrlClick, want.ctrlClick},
		{"number-column Play button", got.playButton, want.playButton},
		{"right-click menu", got.menu, want.menu},
		{"click the artist", got.artist, want.artist},
		{"double-click the artist", got.artistDouble, want.artistDouble},
		{"Shift+click the artist", got.artistShift, want.artistShift},
		{"click the album", got.album, want.album},
		{"Shift+click the album", got.albumShift, want.albumShift},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.what, c.got, c.want)
		}
	}
}
