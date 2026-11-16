package library

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The interaction rule under test: choosing a result opens it; Play plays; a
// track list's row plays (double-click or Enter); a modifier never does
// something a visible cue does not hint at. Each test drives one Library page
// per item kind and pins what every gesture does today. A line marked DEPARTS
// is where today's behaviour does not follow the rule.

// gesture is what a user does to an item.
type gesture string

const (
	gClick      gesture = "click"
	gDouble     gesture = "double-click"
	gEnter      gesture = "Enter"
	gShiftEnter gesture = "Shift+Enter"
	gSpace      gesture = "Space"
	gPlay       gesture = "Play control"
	gRight      gesture = "right-click"
)

var allGestures = []gesture{gClick, gDouble, gEnter, gShiftEnter, gSpace, gPlay, gRight}

// iaCtx is a Library page and how to tell what a gesture did to it.
type iaCtx struct {
	r     *pageRig
	start string
	// played says what was played so far ("" for nothing).
	played func() string
}

// did is what happened so far: the page it went to and what played.
func (x *iaCtx) did() string {
	time.Sleep(80 * time.Millisecond) // playback is asked off the main thread
	x.r.tt.Drain()
	var parts []string
	if p := x.r.router.Location(); p != x.start {
		parts = append(parts, "nav "+p)
	}
	if p := x.played(); p != "" {
		parts = append(parts, "play "+p)
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// focusOn tabs until the focus is on the element named label.
func (x *iaCtx) focusOn(label string) bool {
	for range 60 {
		x.r.tt.Key(0, ui.KeyTab)
		x.r.tt.Drain()
		for _, a := range x.r.tt.AccessAll() {
			if a.Focused && (a.Name == label || strings.HasPrefix(a.Name, label+" ")) {
				return true
			}
		}
	}
	return false
}

// do performs a gesture on the element named label and says what it did.
func (x *iaCtx) do(g gesture, label string, track bool) string {
	x.r.t.Helper()
	tt := x.r.tt
	n, ok := tt.Find(label)
	if !ok {
		x.r.t.Fatalf("no %q; texts %q", label, tt.Texts())
	}
	cx, cy := n.X+n.W/2, n.Y+n.H/2
	if track {
		cx = n.X + 100 // the title, not the artist or album link beside it
	}
	switch g {
	case gClick, gDouble:
		tt.ClickAt(cx, cy)
		tt.Drain()
		if g == gDouble {
			tt.ClickAt(cx, cy)
			tt.Drain()
		}
	case gEnter, gShiftEnter, gSpace:
		if !x.focusOn(label) {
			return "not focusable"
		}
		switch g {
		case gEnter:
			tt.Key(0, ui.KeyEnter)
		case gShiftEnter:
			tt.Key(ui.Shift, ui.KeyEnter)
		case gSpace:
			tt.Key(0, ui.KeySpace)
		}
		tt.Drain()
	case gPlay:
		tt.Move(cx, cy) // a play control shows while its row or tile is hovered
		tt.Drain()
		if err := tt.Click("Play " + label); err != nil {
			return "no play control"
		}
		tt.Drain()
	case gRight:
		tt.RightClickAt(cx, cy)
		tt.Drain()
		tt.Drain()
		for _, a := range tt.AccessAll() {
			if a.Role == ui.RoleMenu || a.Role == ui.RoleMenuItem {
				return "menu"
			}
		}
		return "no menu"
	}
	return x.did()
}

// cue is the role of the element named label for assistive technology: what a
// link, a button, a group or a row tells a user it will do.
func (x *iaCtx) cue(label string) string {
	named := func(name string) bool {
		return name == label || name == label+" Play" || name == label+" Pause" || strings.HasPrefix(name, label+", ")
	}
	for _, role := range []struct {
		r    ui.Role
		name string
	}{{ui.RoleLink, "link"}, {ui.RoleButton, "button"}, {ui.RoleGroup, "group"}, {ui.RoleListItem, "listitem"}, {ui.RoleRow, "row"}} {
		for _, a := range x.r.tt.AccessAll() {
			if a.Role == role.r && named(a.Name) {
				return role.name
			}
		}
	}
	return "none"
}

// engineIDs is a play of the fake engine as its catalogue IDs.
func engineIDs(r *pageRig) func() string {
	return func() string {
		if ids := r.h.Engine.Played(); len(ids) > 0 {
			return strings.Join(ids, ",")
		}
		return ""
	}
}

// itemPlays registers the album and playlist players, as the app does, and
// says what they played: "album:l.a3".
func itemPlays(r *pageRig) func() string {
	var got []string
	for _, kind := range []string{services.ItemAlbum, services.ItemPlaylist} {
		r.h.Env.Players.Register(kind, func(it services.Item) { got = append(got, it.Kind+":"+it.ID) })
	}
	return func() string { return strings.Join(got, ",") }
}

// surface is one Library page, over a library that holds the items.
type surface struct {
	name string
	// track: the items are rows of a track list, whose middle is a link to
	// the artist or album.
	track bool
	// items are the names of the items to drive, by kind.
	items map[string]string
	build func(t *testing.T) *iaCtx
}

// routes makes the pages of the library's items served.
func routes(l *library) {
	l.env.HasRoute = func(p string) bool {
		return strings.HasPrefix(p, "/album/") || strings.HasPrefix(p, "/playlist/") ||
			strings.HasPrefix(p, "/music-video/") || strings.HasPrefix(p, "/artist/")
	}
}

func surfaces() []surface {
	return []surface{
		{"songs", true, map[string]string{"song": "Apple Orchard song 1"}, func(t *testing.T) *iaCtx {
			r := newSongsRig(t, songsPath, routes)
			r.tt.Until("the songs", r.tt.Shows("14 songs", "Apple Orchard song 1"))
			return &iaCtx{r: r, start: songsPath, played: engineIDs(r)}
		}},
		{"favourites", true, map[string]string{"song": "Blue Hour song 2"}, func(t *testing.T) *iaCtx {
			r := newFavouritesRig(t, routes)
			r.tt.Until("no favourites", r.tt.Shows("No favourite songs yet"))
			favouriteSong(r, 1, 0, 10)
			favouriteSong(r, 4, 1, 20)
			r.h.Env.Bus.Emit(services.EventItemState, services.ItemState{Kind: "song", ID: "x"})
			r.tt.Until("two favourites", r.tt.Shows("2 songs", "Blue Hour song 2"))
			return &iaCtx{r: r, start: favouritePath, played: engineIDs(r)}
		}},
		{"history", true, map[string]string{"song": "Alpha"}, func(t *testing.T) *iaCtx {
			r, _, rec := historyRig(t, fixed(twoDays()))
			routes(r.l)
			r.tt.Until("the days", r.tt.Shows("Today", "Alpha", "Beta"))
			return &iaCtx{r: r, start: historyPath, played: func() string {
				rec.mu.Lock()
				defer rec.mu.Unlock()
				var out []string
				for i := range rec.refs {
					out = append(out, fmt.Sprintf("%d songs from %d", len(rec.refs[i]), rec.start[i]))
				}
				return strings.Join(out, ",")
			}}
		}},
		{"albums", false, map[string]string{"album": "Mango Season"}, func(t *testing.T) *iaCtx {
			r := newGridRig(t, albumsPath, nil, uitest.Albums(sevenAlbums...))
			r.tt.Until("the albums", r.tt.Shows("7 albums", "Mango Season"))
			return &iaCtx{r: r.pageRig, start: albumsPath, played: itemPlays(r.pageRig)}
		}},
		{"playlists", false, map[string]string{"playlist": "Focus"}, func(t *testing.T) *iaCtx {
			r := newGridRig(t, playlistsPath, nil)
			r.tt.Until("the playlists", r.tt.Shows("3 playlists", "Focus"))
			return &iaCtx{r: r.pageRig, start: playlistsPath, played: itemPlays(r.pageRig)}
		}},
		{"recentlyAdded", false, map[string]string{"album": "Fieldnotes", "playlist": "Focus"}, func(t *testing.T) *iaCtx {
			r := newGridRigH(t, recentPath, 2400, nil)
			r.tt.Until("the entries", r.tt.Shows("27 albums and playlists", "Fieldnotes", "Focus"))
			return &iaCtx{r: r.pageRig, start: recentPath, played: itemPlays(r.pageRig)}
		}},
		{"videos", false, map[string]string{"video": "Video 0"}, func(t *testing.T) *iaCtx {
			r, plays := newVideosRig(t, videoFixtures(3), true)
			// The player module's actions for a music video (the app registers them beside the library's).
			r.h.Env.Actions.Register(kit.ActionDef{
				ID: "test.play-next.video", Target: kit.KindMusicVideo, Group: kit.GroupQueue, Order: 10,
				Label: func(*kit.ActionContext) string { return "Play next" }, Run: func(*kit.ActionContext) {},
			})
			r.tt.Until("the videos", r.tt.Shows("3 music videos", "Video 0"))
			return &iaCtx{r: r.pageRig, start: videosPath, played: func() string {
				if n := plays.played(); n > 0 {
					return fmt.Sprint(n, " video queue")
				}
				return ""
			}}
		}},
		{"videosNoPlayer", false, map[string]string{"video": "Video 0"}, func(t *testing.T) *iaCtx {
			r, plays := newVideosRig(t, videoFixtures(3), false)
			r.tt.Until("the videos", r.tt.Shows("3 music videos", "Video 0"))
			return &iaCtx{r: r.pageRig, start: videosPath, played: func() string {
				if n := plays.played(); n > 0 {
					return fmt.Sprint(n, " video queue")
				}
				return ""
			}}
		}},
	}
}

// outcomes is what each gesture does to one item, and what its cue is.
type outcomes struct {
	click, double, enter, shiftEnter, space, play, menu, cue string
}

func (o outcomes) of(g gesture) string {
	switch g {
	case gClick:
		return o.click
	case gDouble:
		return o.double
	case gEnter:
		return o.enter
	case gShiftEnter:
		return o.shiftEnter
	case gSpace:
		return o.space
	case gRight:
		return o.menu
	}
	return o.play
}

// libraryWant is what each gesture does to an item, by page and kind, as the code behaves today.
var libraryWant = map[string]map[string]outcomes{
	"songs": {
		"song": {
			click:      "nothing",
			double:     "play 10010",
			enter:      "play 10010",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing on a focused track row, though Enter and a double-click play it and Space plays on Home's item rows
			space: "nothing",
			play:  "play 10010",
			menu:  "menu",
			cue:   "row",
		},
	},
	"favourites": {
		"song": {
			click:      "nothing",
			double:     "play 10041",
			enter:      "play 10041",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing on a focused track row, though Enter and a double-click play it and Space plays on Home's item rows
			space: "nothing",
			play:  "play 10041",
			menu:  "menu",
			cue:   "row",
		},
	},
	"history": {
		"song": {
			click:      "nothing",
			double:     "play 2 songs from 0",
			enter:      "play 2 songs from 0",
			shiftEnter: "nothing",
			// DEPARTS: Space does nothing on a focused track row, though Enter and a double-click play it and Space plays on Home's item rows
			space: "nothing",
			play:  "play 2 songs from 0",
			menu:  "menu",
			cue:   "row",
		},
	},
	"albums": {
		"album": {
			click:      "nav /album/l.a3",
			double:     "nav /album/l.a3",
			enter:      "nav /album/l.a3",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play album:l.a3",
			menu:       "menu",
			cue:        "link",
		},
	},
	"playlists": {
		"playlist": {
			click:      "nav /playlist/p.a1",
			double:     "nav /playlist/p.a1",
			enter:      "nav /playlist/p.a1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play playlist:p.a1",
			menu:       "menu",
			cue:        "link",
		},
	},
	"recentlyAdded": {
		"album": {
			click:      "nav /album/l.a23",
			double:     "nav /album/l.a23",
			enter:      "nav /album/l.a23",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play album:l.a23",
			menu:       "menu",
			cue:        "link",
		},
		"playlist": {
			click:      "nav /playlist/p.a1",
			double:     "nav /playlist/p.a1",
			enter:      "nav /playlist/p.a1",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play playlist:p.a1",
			menu:       "menu",
			cue:        "link",
		},
	},
	"videos": {
		"video": {
			click:      "nav /music-video/cat0",
			double:     "nav /music-video/cat0",
			enter:      "nav /music-video/cat0",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "play 1 video queue",
			menu:       "menu",
			cue:        "link",
		},
	},
	"videosNoPlayer": {
		"video": {
			click:      "nav /music-video/cat0",
			double:     "nav /music-video/cat0",
			enter:      "nav /music-video/cat0",
			shiftEnter: "nothing",
			space:      "nothing",
			play:       "no play control",
			menu:       "no menu",
			cue:        "link",
		},
	},
}

func TestInteractionLibraryGestures(t *testing.T) {
	for _, s := range surfaces() {
		for _, kind := range slices.Sorted(maps.Keys(libraryWant[s.name])) {
			want := libraryWant[s.name][kind]
			label := s.items[kind]
			t.Run(s.name+"/"+kind, func(t *testing.T) {
				for _, g := range allGestures {
					x := s.build(t)
					if g == gClick {
						if got := x.cue(label); got != want.cue {
							t.Errorf("%s of a %s: its cue is %q, want %q", s.name, kind, got, want.cue)
						}
					}
					if got := x.do(g, label, s.track); got != want.of(g) {
						t.Errorf("%s of a %s on %s: did %q, want %q", g, kind, s.name, got, want.of(g))
					}
				}
			})
		}
	}
}

// The Songs page's own Play and Shuffle start the library from its first song
// (or a random one) as the queue.
func TestInteractionLibrarySongsHead(t *testing.T) {
	for _, name := range []string{"Play your songs", "Shuffle"} {
		r := newSongsRig(t, songsPath, routes)
		r.tt.Until("the songs", r.tt.Shows("14 songs", "Apple Orchard song 1"))
		x := &iaCtx{r: r, start: songsPath, played: engineIDs(r)}
		got := x.do(gClick, name, false)
		if name == "Play your songs" && got != "play 10010" {
			t.Errorf("click on %q did %q, want %q", name, got, "play 10010")
		}
		if !strings.HasPrefix(got, "play ") {
			t.Errorf("click on %q did %q, want a play", name, got)
		}
	}
}

// An artist's row in the master list is chosen, not opened: it shows the
// artist in the detail beside it (and in the URL), and has no Play control of
// its own: the detail's Play plays the artist's songs.
func TestInteractionLibraryArtists(t *testing.T) {
	const row = "Mira Oyelaran"
	chosen := artistsPath + "?artist=r.a2"
	open := func() (*pageRig, *iaCtx) {
		r := newPageRig(t, artistsPath, 1132, 900, routes)
		r.tt.Until("the list", r.tt.Shows(row, "Albums by Dara Finch in your library"))
		return r, &iaCtx{r: r, start: artistsPath, played: engineIDs(r)}
	}
	for _, g := range []gesture{gClick, gDouble} {
		r, x := open()
		if _, ok := r.tt.Find("Play " + row); ok {
			t.Errorf("a row of the list has a Play control")
		}
		if got := x.do(g, row, false); got != "nav "+chosen {
			t.Errorf("%s on an artist's row did %q, want it chosen (%q)", g, got, "nav "+chosen)
		}
		r.tt.Until("the detail", r.tt.Shows("Albums by "+row+" in your library", "Play "+row))
	}
	// Enter and Shift+Enter on the list, which a click gave the keys, go into the
	// detail; Space is not handled.
	for g, want := range map[gesture]string{
		gEnter:      chosen + " list focused false",
		gShiftEnter: chosen + " list focused false",
		gSpace:      chosen + " list focused true",
	} {
		r, _ := open()
		if err := r.tt.Click(row); err != nil {
			t.Fatal(err)
		}
		r.tt.Until("the detail", r.tt.Shows("Albums by "+row+" in your library"))
		switch g {
		case gEnter:
			r.tt.Key(0, ui.KeyEnter)
		case gShiftEnter:
			r.tt.Key(ui.Shift, ui.KeyEnter)
		case gSpace:
			r.tt.Key(0, ui.KeySpace)
		}
		for range 6 {
			r.tt.Drain()
		}
		got := fmt.Sprintf("%s list focused %v", r.router.Location(), r.tt.Focused("Artists in your library"))
		if got != want {
			t.Errorf("%s on the chosen artist's row: %q, want %q", g, got, want)
		}
	}
	// The detail's Play plays the artist; the row is an option of a list, not a
	// link or a button.
	r, x := open()
	if err := r.tt.Click(row); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the detail", r.tt.Shows("Play "+row))
	if err := r.tt.Click("Play " + row); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the artist to play", func() bool { return len(r.h.Engine.Played()) > 0 })
	if got := x.did(); !strings.HasPrefix(got, "nav "+chosen+"; play ") {
		t.Errorf("Play in the detail did %q", got)
	}
	var cue string
	for _, a := range r.tt.AccessAll() {
		if strings.HasPrefix(a.Name, row+", ") && a.Role == ui.RoleOption {
			cue = fmt.Sprintf("option selected %v focusable %v", a.Selected, a.Focusable)
		}
	}
	if cue != "option selected true focusable false" {
		t.Errorf("the chosen artist's row is %q", cue)
	}
}
