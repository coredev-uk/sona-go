package player

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// withActions registers the player's own item actions and a Go to album, as
// the shell does for every module.
func withActions(r *rig) {
	reg := r.env.Actions // the harness's, which the menus already hold
	reg.Register(r.p.actions()...)
	reg.Register(kit.ActionDef{
		ID: "test.go-to-album", Target: kit.KindSong, Group: kit.GroupItem, Order: 10,
		Label: func(*kit.ActionContext) string { return "Go to album" },
		Run:   func(a *kit.ActionContext) { a.Navigate("/album/" + a.Item().Song().AlbumID) },
	})
}

func TestBarSongOpensTheSongMenuOnRightClick(t *testing.T) {
	r := newRig(t, true)
	withActions(r)
	nav := routed(r, true) // the title and the artist are links, as in the app
	r.env.Actions.SetNavigator(nav.Push)
	r.seed(func(s *services.PlaybackState) { s.Current.AlbumID, s.Current.ArtistID = "al.9", "ar.7" })

	// A link takes the pointer from the song around it: it opens the menu itself.
	for _, from := range []string{"The Artist", "Song Title"} {
		b, ok := r.tt.Find(from)
		if !ok {
			t.Fatalf("no %q in %q", from, r.tt.Texts())
		}
		r.tt.RightClickAt(b.X+b.W/2, b.Y+b.H/2)
		r.tt.Frame()
		if !r.tt.HasText("Play next") {
			t.Fatalf("right-click on the %s link opened no menu: %q", from, r.tt.Texts())
		}
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Frame()
		if len(nav.Pushed) != 0 {
			t.Fatalf("right-click on the %s link followed it: %q", from, nav.Pushed)
		}
	}
	b, _ := r.tt.Find("Song Title")
	r.tt.RightClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.Frame()
	for _, w := range []string{"Play next", "Play last", "Go to album"} {
		if !r.tt.HasText(w) {
			t.Fatalf("the menu lacks %q: %q", w, r.tt.Texts())
		}
	}
	r.tt.MustClick("Go to album")
	r.tt.Frame()
	if len(nav.Pushed) != 1 || nav.Pushed[0] != "/album/al.9" {
		t.Errorf("pushed %q", nav.Pushed)
	}

	// The cover too, and a queue action reaches the transport.
	c, _ := r.tt.Find("Open now playing")
	r.tt.RightClickAt(c.X+c.W/2, c.Y+c.H/2)
	r.tt.Frame()
	if err := r.tt.Click("Play next"); err != nil {
		t.Fatalf("no Play next in the cover's menu: %v (%q)", err, r.tt.Texts())
	}
	if got := r.fake.Wait(t, 1); got[0] != "playnext" {
		t.Errorf("calls %v", got)
	}
}

func TestBarSongMenuOpensFromTheKeyboard(t *testing.T) {
	r := newRig(t, true)
	withActions(r)
	r.seed()
	for range 3 { // the cover is the bar's first stop
		if r.tt.Focused("Open now playing") {
			break
		}
		r.tt.Key(0, ui.KeyTab)
		r.tt.Frame()
	}
	if !r.tt.Focused("Open now playing") {
		t.Fatal("Tab did not reach the cover")
	}
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Frame()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if len(tr.Labelled(ui.RoleMenuItem, "Play next")) == 0 {
		t.Fatalf("Shift+F10 did not open the song menu:\n%s", tr.Dump())
	}
}

func TestBarMenuOfStationsAndNothingPlaying(t *testing.T) {
	r := newRig(t, true)
	withActions(r)
	r.seed(func(s *services.PlaybackState) { s.Current = nil })
	r.tt.RightClickAt(100, 370)
	r.tt.Frame()
	if r.tt.HasText("Play next") {
		t.Error("a menu with nothing playing")
	}
}

// npActions registers what withActions does, for Now Playing's rig.
func npActions(r *npRig) {
	r.h.Env.Actions.Register(r.p.actions()...)
	r.h.Env.Actions.Register(kit.ActionDef{
		ID: "test.go-to-album", Target: kit.KindSong, Group: kit.GroupItem, Order: 10,
		Label: func(*kit.ActionContext) string { return "Go to album" },
		Run:   func(a *kit.ActionContext) { a.Navigate("/album/" + a.Item().Song().AlbumID) },
	})
	queueOver(r.h.Env, newFake())
}

func TestNowPlayingCoverTitleAndMoreOpenTheSongMenu(t *testing.T) {
	r := newNPRig(t)
	npActions(r)
	r.play(30_000)
	r.open("")

	for _, from := range []string{"Song Title", "The Album cover"} {
		b, ok := r.tt.Find(from)
		if !ok {
			t.Fatalf("no %q in %q", from, r.tt.Texts())
		}
		r.tt.RightClickAt(b.X+b.W/2, b.Y+b.H/2)
		r.tt.Frame()
		r.tt.Frame()
		if !r.tt.HasText("Play next") || !r.tt.HasText("Go to album") {
			t.Fatalf("right-click on %q opened no song menu: %q", from, r.tt.Texts())
		}
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Frame()
		r.tt.Frame()
		if r.tt.HasText("Go to album") {
			t.Fatalf("Escape left the menu of %q open", from)
		}
	}

	// The More button is the menu's visible way in: a menu button that says
	// whether its menu is open.
	tr := access.Read(t, r.tt.Tester)
	if got := tr.Labelled(ui.RoleMenuButton, "More for Song Title"); len(got) != 1 {
		t.Fatalf("no menu button \"More for Song Title\":\n%s", tr.Dump())
	}
	r.tt.MustClick("More for Song Title")
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Play next") {
		t.Fatalf("the More button opened nothing: %q", r.tt.Texts())
	}
	r.tt.MustClick("Play next")
	r.tt.Frame()
	if r.tt.HasText("Go to album") {
		t.Error("the menu stayed open after a choice")
	}
}

// Now Playing covers the page: a page a chosen action opens closes it first,
// and its artist link opens the menu on a right-click too.
func TestNowPlayingMenuNavigationClosesTheView(t *testing.T) {
	r := newNPRig(t)
	npActions(r)
	nav := &uitest.Nav{}
	r.h.Env.Nav = nav
	r.h.Env.HasRoute = func(string) bool { return true }
	r.h.Env.Actions.SetNavigator(nav.Push)
	r.play(30_000)
	r.open("")

	b, ok := r.tt.Find("The Artist")
	if !ok {
		t.Fatalf("no artist link in %q", r.tt.Texts())
	}
	r.tt.RightClickAt(b.X+b.W/2, b.Y+b.H/2)
	r.tt.Frame()
	r.tt.Frame()
	if !r.tt.HasText("Go to album") {
		t.Fatalf("right-click on the artist link opened no menu: %q", r.tt.Texts())
	}
	if len(nav.Pushed) != 0 {
		t.Fatalf("right-click followed the link: %q", nav.Pushed)
	}
	r.tt.MustClick("Go to album")
	r.tt.Frame()
	r.tt.Frame()
	if len(nav.Pushed) != 1 || nav.Pushed[0] != "/album/al1" {
		t.Errorf("pushed %q", nav.Pushed)
	}
	if r.p.ui.overlay != overlayNone {
		t.Error("Now Playing stayed open over the page the action opened")
	}
}
