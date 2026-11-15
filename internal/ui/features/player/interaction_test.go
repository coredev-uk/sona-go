package player

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the queue panel and the
// player bar do today for each gesture and each kind. The rule being reviewed:
// choosing a result opens it; Play plays; a track list's row plays
// (double-click or Enter); a modifier never does something a visible cue does
// not hint at. Where today's behaviour departs from it, the assertion carries
// a "DEPARTS:" comment, to be found (and flipped) when the behaviour is fixed.

// iaGest is what each gesture on one control did, as the surface's effects
// word it.
type iaGest struct {
	click, dbl, enter, shiftEnter, space, right string
}

// iaWant fails for each gesture whose effect is not the one pinned.
func iaWant(t *testing.T, what string, got, want iaGest) {
	t.Helper()
	for _, c := range []struct{ gesture, got, want string }{
		{"click", got.click, want.click},
		{"double-click", got.dbl, want.dbl},
		{"Enter", got.enter, want.enter},
		{"Shift+Enter", got.shiftEnter, want.shiftEnter},
		{"Space", got.space, want.space},
		{"right-click", got.right, want.right},
	} {
		if c.got != c.want {
			t.Errorf("%s, %s: %s, want %s", what, c.gesture, c.got, c.want)
		}
	}
}

// iaMark is the label of an action every kind of item lists, so that a menu
// that opened can be told from none without asserting what a menu holds.
const iaMark = "Menu mark"

func iaMarkActions(env *module.Env) {
	for _, k := range []kit.ItemKind{kit.KindSong, kit.KindStation, kit.KindMusicVideo} {
		env.Actions.Register(kit.ActionDef{
			ID: "test.ia." + string(k), Target: k, Group: kit.GroupItem, Order: 1,
			Label: func(*kit.ActionContext) string { return iaMark },
			Run:   func(*kit.ActionContext) {},
		})
	}
}

// iaTab presses Tab until the control called label has the focus, which the
// accessibility tree names.
func iaTab(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	for range 80 {
		if tr := access.Read(t, tt); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		tt.Key(0, ui.KeyTab)
		tt.Frame()
	}
	t.Fatalf("Tab never reached %q", label)
}

// The queue panel.

// iaQueue is the queue panel over its fake, with the effects of a gesture
// worded: what reached the playback service, which page was pushed, whether a
// menu opened.
type iaQueue struct {
	t *testing.T
	r *queueRig
	// calls and pushes are how many there were before the gesture.
	calls, pushes int
}

// iaQueueRig seeds five songs, the first playing, with the song at index 2
// made a row of the given kind: "song", "auto" (AutoPlay's), "video"; "current"
// is the playing row itself.
func iaQueueRig(t *testing.T, kind string) *iaQueue {
	t.Helper()
	r := newQueueRig(t, false)
	r.h.Env.HasRoute = func(string) bool { return true }
	r.seed(5, 0, func(s *services.PlaybackState, q *services.QueueState) {
		switch kind {
		case "auto":
			q.Items[2].Auto, q.Items[3].Auto, q.Items[4].Auto = true, true, true
			s.AutoPlay = true
			q.Station = &services.QueueStation{ID: "st.1", Name: "Chill", AutoPlay: true}
		case "video":
			q.Items[2].Video, q.Items[2].CatalogID = true, "mv.3"
		}
	})
	return &iaQueue{t: t, r: r}
}

// fx is what the gestures since mark asked of the app: "nothing" when none did.
func (q *iaQueue) fx() string {
	for range 8 {
		q.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	out = append(out, q.r.fake.Snapshot()[q.calls:]...)
	for _, p := range q.r.nav.Pushed[q.pushes:] {
		out = append(out, "nav "+p)
	}
	if q.r.p.queue.menu.IsOpen() {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// mark is the point from which a gesture's effects count.
func (q *iaQueue) mark() {
	q.calls, q.pushes = len(q.r.fake.Snapshot()), len(q.r.nav.Pushed)
}

// gesture does one gesture on the row called label, in this rig, and says what it
// did (a menu a gesture left open is closed first). A key is pressed on the row
// after a click has focused it.
func (q *iaQueue) gesture(g, label string) string {
	q.t.Helper()
	if q.r.p.queue.menu.IsOpen() {
		q.r.p.queue.menu.Close()
	}
	q.r.tt.Frame()
	x, y := q.r.at(q.t, label)
	key := func(mods ui.Modifiers, k ui.Key) string {
		q.r.focusRow(q.t, label)
		q.mark()
		q.r.tt.Key(mods, k)
		return q.fx()
	}
	switch g {
	case "click":
		q.mark()
		q.r.tt.ClickAt(x, y)
		return q.fx()
	case "double-click":
		q.mark()
		q.r.tt.ClickAt(x, y)
		q.r.tt.ClickAt(x, y)
		return q.fx()
	case "right-click":
		q.mark()
		q.r.tt.RightClickAt(x, y)
		return q.fx()
	case "Enter":
		return key(0, ui.KeyEnter)
	case "Shift+Enter":
		return key(ui.Shift, ui.KeyEnter)
	case "Space":
		return key(0, ui.KeySpace)
	case "Menu key":
		return key(0, ui.KeyContextMenu)
	case "Shift+F10":
		return key(ui.Shift, ui.KeyF10)
	}
	q.t.Fatalf("no gesture %q", g)
	return ""
}

// A row of the queue (a song of the user's, one of AutoPlay's, a music video)
// is a track list's row: a double-click or Enter plays from it (the service is
// asked to jump to it), a click only selects it, and a right-click or the menu
// keys open its menu. Nothing else does anything, a modifier on Enter and Space
// included.
func TestInteractionQueueRows(t *testing.T) {
	row := iaGest{click: "nothing", dbl: "jump 2", enter: "jump 2", shiftEnter: "nothing", space: "nothing", right: "menu"}
	for _, c := range []struct {
		kind, label string
		want        iaGest
	}{
		{"song", "Song 3, Artist 3", row},
		{"auto", "Song 3, Artist 3", row},
		{"video", "Song 3, Artist 3, music video", row},
	} {
		q := iaQueueRig(t, c.kind)
		for _, g := range []struct{ name, want string }{
			{"click", c.want.click}, {"double-click", c.want.dbl}, {"Enter", c.want.enter}, {"Shift+Enter", c.want.shiftEnter},
			{"Space", c.want.space}, {"right-click", c.want.right}, {"Menu key", "menu"}, {"Shift+F10", "menu"},
		} {
			if got := q.gesture(g.name, c.label); got != g.want {
				t.Errorf("%s row, %s: %s, want %s", c.kind, g.name, got, g.want)
			}
		}
	}
}

// The song playing has a row of its own ("Now playing"): a double-click or
// Enter on it plays it again from the start (the service jumps to its own
// index); it has no menu and no Remove.
func TestInteractionQueueNowPlayingRow(t *testing.T) {
	q := iaQueueRig(t, "current")
	for _, c := range []struct{ gesture, want string }{
		{"click", "nothing"},
		{"double-click", "jump 0"}, // restarts the song: Jump plays queue[index] from 0, the playing one too
		{"Enter", "jump 0"},
		{"Shift+Enter", "nothing"},
		{"Space", "nothing"},
		// DEPARTS: right-click opens no menu on the Now playing row, though the same song's bar and cover open its menu and every other row opens one.
		{"right-click", "nothing"},
		// DEPARTS: nor do the menu keys, so the playing song's menu is out of the keyboard's reach here.
		{"Menu key", "nothing"},
		{"Shift+F10", "nothing"},
	} {
		if got := q.gesture(c.gesture, "Song 1, Artist 1"); got != c.want {
			t.Errorf("Now playing row, %s: %s, want %s", c.gesture, got, c.want)
		}
	}
}

// A song's title and artist in a row are plain text, not links: choosing them
// is choosing the row, and the page they could open (the album, the artist) is
// reached only through the menu. A music video's title is a link to its page
// (a pointer shortcut: the row stays the list's one Tab stop, and Enter plays).
func TestInteractionQueueRowTitleAndArtist(t *testing.T) {
	q := iaQueueRig(t, "song")
	for _, name := range []string{"Song 3", "Artist 3"} {
		if info, ok := q.r.tt.Access(name); ok && info.Role == ui.RoleLink {
			t.Errorf("%s is a link", name)
		}
		q.mark()
		if err := q.r.tt.Click(name); err != nil {
			t.Fatal(err)
		}
		// DEPARTS: the same song's title and artist are links in the player bar (to its album and to the artist), here they are plain text that opens nothing.
		if got := q.fx(); got != "nothing" {
			t.Errorf("click on the %s of a song row: %s, want nothing", name, got)
		}
	}

	v := iaQueueRig(t, "video")
	if info, ok := v.r.tt.Access("Song 3"); !ok || info.Role != ui.RoleLink {
		t.Errorf("a music video's title: role %v (found %v), want a link", info.Role, ok)
	}
	v.mark()
	if err := v.r.tt.Click("Song 3"); err != nil {
		t.Fatal(err)
	}
	// DEPARTS: a music video's title in a row opens its page on a click, though a song's title in the next row opens nothing.
	if got := v.fx(); got != "nav /music-video/mv.3" {
		t.Errorf("click on a music video's title: %s, want its page", got)
	}
	v.mark()
	v.r.focusRow(t, "Song 3, Artist 3, music video")
	v.mark()
	v.r.tt.Key(0, ui.KeyEnter)
	if got := v.fx(); got != "jump 2" {
		t.Errorf("Enter on a music video's row: %s, want a jump", got)
	}

	// Without the page the title is plain text.
	u := iaQueueRig(t, "video")
	u.r.h.Env.HasRoute = func(string) bool { return false }
	u.r.tt.Frame()
	u.mark()
	_ = u.r.tt.Click("Song 3")
	if got := u.fx(); got != "nothing" {
		t.Errorf("click on a music video's title without its page: %s, want nothing", got)
	}
}

// A row's More and Remove buttons show while the pointer is on the row: More
// opens the menu, Remove takes the song out. They are not Tab stops: the row's
// Delete and Shift+F10 do the same from the keyboard.
func TestInteractionQueueRowButtons(t *testing.T) {
	for _, c := range []struct{ button, want string }{
		{"More for Song 3", "menu"},
		{"Remove Song 3 from the queue", "remove 2"},
	} {
		q := iaQueueRig(t, "song")
		x, y := q.r.at(t, "Song 3, Artist 3")
		q.r.tt.Move(x, y)
		q.r.tt.Frame()
		q.mark()
		if err := q.r.tt.Click(c.button); err != nil {
			t.Fatal(err)
		}
		if got := q.fx(); got != c.want {
			t.Errorf("click on %q: %s, want %s", c.button, got, c.want)
		}
	}
	q := iaQueueRig(t, "song")
	for range 40 {
		q.r.tt.Key(0, ui.KeyTab)
		q.r.tt.Frame()
		if tr := access.Read(t, q.r.tt.Tester); tr.Focus != nil && (strings.HasPrefix(tr.Focus.Label, "More for ") || strings.HasPrefix(tr.Focus.Label, "Remove ")) {
			t.Fatalf("Tab stops at %q", tr.Focus.Label)
		}
	}
}

// "Playing from <station>" over the list: the station's name is a link to its
// page, where Sona has it; the link opens the page on a click and is plain text
// without it.
func TestInteractionQueuePlayingFromLink(t *testing.T) {
	r := newQueueRig(t, true)
	r.h.Env.HasRoute = func(string) bool { return true }
	r.seed(3, 0, func(s *services.PlaybackState, q *services.QueueState) {
		q.Station = &services.QueueStation{ID: "st.1", Name: "Chill"}
	})
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleLink, "Chill")
	if !ok {
		t.Fatalf("no link Chill:\n%s", tr.Dump())
	}
	// DEPARTS: the link has no Tab stop, so the station's page cannot be reached from the keyboard here.
	if n.Has(access.Focusable) {
		t.Error("the station link is a Tab stop")
	}
	r.tt.MustClick("Chill")
	if got := r.nav.Pushed; len(got) != 1 || got[0] != "/station/st.1" {
		t.Errorf("click pushed %v", got)
	}
	r.nav.Pushed = nil
	x, y := r.at(t, "Chill")
	r.tt.ClickAt(x-60+3, y)
	r.tt.ClickAt(x-60+3, y)
	if got := r.nav.Pushed; len(got) != 2 {
		t.Errorf("a double-click pushed %v, want the page twice (the router keeps one entry)", got)
	}
}

// The player bar.

// iaBar is the bar over its fake, with a real router so that pushes of one
// page count once, as in the app, and the item menu marked.
type iaBar struct {
	t      *testing.T
	r      *rig
	router *ui.Router
	calls  int
}

// iaBarRig is the bar showing a song 30 s in, shaped by mutate, with every page
// linkable unless routed says not.
func iaBarRig(t *testing.T, routed bool, mutate ...func(*services.PlaybackState)) *iaBar {
	t.Helper()
	r := newRig(t, true)
	withActions(r)
	iaMarkActions(r.env)
	b := &iaBar{t: t, r: r, router: ui.NewRouter("/start")}
	r.env.Nav = b.router
	r.env.HasRoute = func(string) bool { return routed }
	r.env.Actions.SetNavigator(b.router.Push)
	r.seed(mutate...)
	return b
}

// reset closes what a gesture opened, goes back to the start page and forgets
// the calls so far.
func (b *iaBar) reset() {
	b.r.tt.Key(0, ui.KeyEscape)
	b.r.p.ui.closeOverlay()
	b.r.env.Video.Close()
	b.router.Push("/start")
	b.r.tt.Frame()
	b.r.tt.Frame()
	b.calls = len(b.r.fake.Snapshot())
}

func (b *iaBar) fx() string {
	for range 8 {
		b.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if loc := b.router.Location(); loc != "/start" {
		out = append(out, "nav "+loc)
	}
	out = append(out, b.r.fake.Snapshot()[b.calls:]...)
	if b.r.p.ui.overlay != overlayNone {
		out = append(out, "now playing opens")
	}
	if b.r.env.Video.IsOpen() {
		out = append(out, "video opens")
	}
	if b.r.tt.HasText(iaMark) {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// gestures does each gesture on the control called label, each from a clean
// start, and says what it did.
func (b *iaBar) gestures(label string) iaGest {
	b.t.Helper()
	r, ok := b.r.tt.Find(label)
	if !ok {
		b.t.Fatalf("%q is not on screen: %q", label, b.r.tt.Texts())
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	var g iaGest
	b.reset()
	b.r.tt.ClickAt(x, y)
	g.click = b.fx()
	b.reset()
	b.r.tt.ClickAt(x, y)
	b.r.tt.ClickAt(x, y)
	g.dbl = b.fx()
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
		into *string
	}{{0, ui.KeyEnter, &g.enter}, {ui.Shift, ui.KeyEnter, &g.shiftEnter}, {0, ui.KeySpace, &g.space}} {
		b.reset()
		iaTab(b.t, b.r.tt.Tester, label)
		b.r.tt.Key(k.mods, k.key)
		*k.into = b.fx()
	}
	b.reset()
	b.r.tt.RightClickAt(x, y)
	g.right = b.fx()
	b.reset()
	return g
}

// iaSong is the bar's song with the pages its title, artist and album link to.
func iaSong(s *services.PlaybackState) {
	s.Current.AlbumID, s.Current.ArtistID = "al.9", "ar.7"
}

// The cover is a button that opens Now Playing (or the music video, where one
// plays); the press that opens it closes it again, so a double-click leaves it
// shut. It is the bar's first Tab stop and takes the song's menu on a
// right-click.
func TestInteractionBarCover(t *testing.T) {
	// A song.
	b := iaBarRig(t, true, iaSong)
	iaWant(t, "the cover of a song", b.gestures("Open now playing"), iaGest{
		click:      "now playing opens",
		dbl:        "nothing", // the second click closes what the first opened
		enter:      "now playing opens",
		shiftEnter: "nothing",
		space:      "now playing opens",
		right:      "menu",
	})

	// A music video: the cover shows the video.
	v := iaBarRig(t, true, func(s *services.PlaybackState) { s.Current.Video, s.Current.CatalogID = true, "mv.5" })
	iaWant(t, "the cover of a music video", v.gestures("Show video"), iaGest{
		click:      "video opens",
		dbl:        "video opens",
		enter:      "video opens",
		shiftEnter: "nothing",
		space:      "video opens",
		right:      "menu",
	})

	// A live station.
	l := iaBarRig(t, true, func(s *services.PlaybackState) { s.Current.Live = true })
	iaWant(t, "the cover of a live station", l.gestures("Open now playing"), iaGest{
		click: "now playing opens", dbl: "nothing", enter: "now playing opens", shiftEnter: "nothing", space: "now playing opens", right: "menu",
	})
}

// The title is a link to where the playing item has a page: a song to its album
// with the song marked, a station to its page, a music video to its page; a
// local broadcaster's station has none, and nothing is a link where Sona has
// not the page. A link opens on a click, a double-click (the router keeps one
// entry) and Enter; Space and Shift+Enter do nothing; right-click opens the
// item's menu (the link does not take it).
func TestInteractionBarTitle(t *testing.T) {
	link := func(loc string) iaGest {
		return iaGest{click: "nav " + loc, dbl: "nav " + loc, enter: "nav " + loc, shiftEnter: "nothing", space: "nothing", right: "menu"}
	}
	for _, c := range []struct {
		kind   string
		mutate func(*services.PlaybackState)
		want   iaGest
	}{
		{"a song", iaSong, link("/album/al.9?song=i.1")},
		{"a live station", func(s *services.PlaybackState) { s.Current.Live = true }, link("/station/i.1")},
		{"a music video", func(s *services.PlaybackState) { s.Current.Video, s.Current.CatalogID = true, "mv.5" }, link("/music-video/mv.5")},
	} {
		b := iaBarRig(t, true, c.mutate)
		if info, ok := b.r.tt.Access("Song Title"); !ok || info.Role != ui.RoleLink {
			t.Errorf("the title of %s: role %v (found %v), want a link", c.kind, info.Role, ok)
		}
		iaWant(t, "the title of "+c.kind, b.gestures("Song Title"), c.want)
	}

	// No page, no link: the title is text; a right-click still opens the menu.
	for _, c := range []struct {
		kind   string
		routed bool
		mutate func(*services.PlaybackState)
	}{
		{"a broadcaster's station", true, func(s *services.PlaybackState) { s.Current.Live, s.Current.Broadcast = true, true }},
		{"a song with no album page", false, iaSong},
	} {
		b := iaBarRig(t, c.routed, c.mutate)
		if info, ok := b.r.tt.Access("Song Title"); ok && info.Role == ui.RoleLink {
			t.Errorf("the title of %s is a link", c.kind)
		}
		x, y := func() (float32, float32) { r, _ := b.r.tt.Find("Song Title"); return r.X + r.W/2, r.Y + r.H/2 }()
		b.reset()
		b.r.tt.ClickAt(x, y)
		b.r.tt.ClickAt(x, y)
		if got := b.fx(); got != "nothing" {
			t.Errorf("the title of %s, double-click: %s, want nothing", c.kind, got)
		}
	}
}

// The artist and the album under the title are links to their pages where
// Sona has them: a click opens, as the title does.
func TestInteractionBarArtistAndAlbum(t *testing.T) {
	b := iaBarRig(t, true, iaSong)
	for _, c := range []struct{ label, loc string }{{"The Artist", "/artist/ar.7"}, {"The Album", "/album/al.9"}} {
		if info, ok := b.r.tt.Access(c.label); !ok || info.Role != ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a link", c.label, info.Role, ok)
		}
		link := "nav " + c.loc
		iaWant(t, c.label, b.gestures(c.label), iaGest{
			click: link, dbl: link, enter: link, shiftEnter: "nothing", space: "nothing", right: "menu",
		})
	}
}

// Play/Pause is a button of the transport: it toggles on a click, Enter and
// Space (a double-click is two toggles), and does nothing else.
func TestInteractionBarPlayPause(t *testing.T) {
	b := iaBarRig(t, true, iaSong)
	iaWant(t, "Pause", b.gestures("Pause"), iaGest{
		click: "toggle", dbl: "toggle; toggle", enter: "toggle", shiftEnter: "nothing", space: "toggle", right: "nothing",
	})
}

// A song of several artists shows their credit as one menu button, not a link:
// a click, Enter and Space open a chooser of the artists and open no page by
// themselves, Shift+Enter does nothing, and choosing an artist opens theirs.
func TestInteractionBarSeveralArtists(t *testing.T) {
	b := iaBarRig(t, true, func(s *services.PlaybackState) {
		s.Current.Artist, s.Current.ArtistID, s.Current.Artists = stayCredit, "1435848034", stayArtists
		s.Current.AlbumID = "al.9"
	})
	if info, ok := b.r.tt.Access(stayCredit); !ok || info.Role != ui.RoleMenuButton {
		t.Fatalf("the credit: role %v (found %v), want a menu button", info.Role, ok)
	}
	chooser := func() bool {
		_, ok := access.Read(t, b.r.tt.Tester).Find(ui.RoleMenuItem, "Justin Bieber")
		return ok
	}
	r, _ := b.r.tt.Find(stayCredit)
	x, y := r.X+r.W/2, r.Y+r.H/2
	for _, c := range []struct {
		gesture string
		do      func()
		opens   bool
	}{
		{"click", func() { b.r.tt.ClickAt(x, y) }, true},
		{"Enter", func() { iaTab(t, b.r.tt.Tester, stayCredit); b.r.tt.Key(0, ui.KeyEnter) }, true},
		{"Space", func() { iaTab(t, b.r.tt.Tester, stayCredit); b.r.tt.Key(0, ui.KeySpace) }, true},
		{"Shift+Enter", func() { iaTab(t, b.r.tt.Tester, stayCredit); b.r.tt.Key(ui.Shift, ui.KeyEnter) }, false},
	} {
		b.reset()
		c.do()
		b.r.tt.Frame()
		if got := chooser(); got != c.opens {
			t.Errorf("%s on the credit: chooser open %v, want %v", c.gesture, got, c.opens)
		}
		if loc := b.router.Location(); loc != "/start" {
			t.Errorf("%s on the credit went to %q", c.gesture, loc)
		}
	}
	b.reset()
	b.r.tt.ClickAt(x, y)
	b.r.tt.MustClick("Justin Bieber")
	b.r.tt.Pump(3, 50*time.Millisecond)
	if loc := b.router.Location(); loc != "/artist/320569549" {
		t.Errorf("choosing an artist went to %q", loc)
	}
}
