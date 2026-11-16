package artist

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the artist page does
// today for each gesture and each kind. The rule being reviewed: choosing a
// result opens it; Play plays; a track list's row plays (double-click or
// Enter); a modifier never does something a visible cue does not hint at. Where
// today's behaviour departs from it, the assertion carries a "DEPARTS:"
// comment, to be found (and flipped) when the behaviour is fixed.

// iaGest is what each gesture on one control did, as iaFx words it.
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

// iaJoin is several effects in the order they happened.
func iaJoin(fx ...string) string { return strings.Join(fx, "; ") }

// How a control takes the keyboard.
const (
	iaTabStop = iota // Tab reaches it
	iaRow            // a track row: one stop of its list, focused by a click
)

// iaPage drives the gestures of an artist page from a clean start and says what
// each did: a page pushed, a play of songs through the queue, an item played
// through the item players, a menu opened, rows selected.
type iaPage struct {
	r *rig
	// path is the page itself, where each gesture starts.
	path string
	// items are the items the item players were asked to play.
	items *[]string
	// list, when set, is the release list whose selection is worded.
	list string
	// name is the artist, which shows once the page is laid out.
	name string
	// scrolled is whether a Tab may have scrolled the page since it was last at
	// its top.
	scrolled *bool
}

// iaPageOf is the page of a rig whose album players are noted.
func iaPageOf(r *rig) iaPage {
	items := new([]string)
	r.h.Env.Players.Register(services.ItemAlbum, func(it services.Item) { *items = append(*items, "item "+it.Kind+":"+it.ID) })
	return iaPage{r: r, path: "/artist/657515", items: items, name: "Radiohead", scrolled: new(bool)}
}

func (p iaPage) fx() string {
	for range 12 {
		p.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if path := p.r.router.Path(); path != p.path {
		out = append(out, "nav "+path)
	}
	p.r.play.mu.Lock()
	for _, pl := range p.r.play.plays {
		ids := make([]string, len(pl.refs))
		for i, ref := range pl.refs {
			ids[i] = ref.ID
		}
		start := " start=" + strconv.Itoa(pl.start)
		if p.r.play.shuffle {
			start = "" // a shuffle starts from a random song
		}
		out = append(out, "play "+strconv.Itoa(len(ids))+" songs"+start+" from="+pl.from.Kind+":"+pl.from.ID)
	}
	if p.r.play.shuffle {
		out = append(out, "shuffle on")
	}
	p.r.play.mu.Unlock()
	out = append(out, *p.items...)
	if p.list != "" {
		if n := p.r.a.page.list(p.list).sel.Len(); n > 0 {
			out = append(out, "selected "+strconv.Itoa(n))
		}
	}
	if n := len(access.Read(p.r.t, p.r.tt.Tester).Roles(ui.RoleMenuItem)); n > 0 {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// clear forgets what a gesture did, without leaving the page.
func (p iaPage) clear() {
	p.r.play.mu.Lock()
	p.r.play.plays, p.r.play.shuffle = nil, false
	p.r.play.mu.Unlock()
	*p.items = nil
	if p.list != "" {
		p.r.a.page.list(p.list).sel.Clear()
	}
}

// reset closes what a gesture opened and forgets what it did. A page that was
// opened has rebuilt: it waits for it to be laid out again, and scrolls it to
// its top (Tab scrolls to the control it reaches).
func (p iaPage) reset() {
	p.r.t.Helper()
	p.r.tt.Key(0, ui.KeyEscape)
	left := p.r.router.Path() != p.path
	p.r.router.Push(p.path)
	p.clear()
	if left || *p.scrolled {
		p.r.tt.Until("the page", func() bool { return p.r.router.Path() == p.path && p.r.tt.HasText(p.name) })
		p.r.tt.Scroll(600, 400, 0, -10000)
		p.r.tt.Settle()
		*p.scrolled = false
	}
	p.r.tt.Frame()
}

// tab presses Tab until the control called label has the focus, which the
// accessibility tree names.
func (p iaPage) tab(label string) {
	p.r.t.Helper()
	*p.scrolled = true
	for range 160 {
		if tr := access.Read(p.r.t, p.r.tt.Tester); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		p.r.tt.Key(0, ui.KeyTab)
	}
	p.r.t.Fatalf("Tab never reached %q", label)
}

// gestures runs each gesture on the control called label (a row is focused by
// a click first, at its own area).
func (p iaPage) gestures(label string, how int) iaGest {
	p.r.t.Helper()
	// at is where the control is now: the page lays out again after it was left.
	at := func() (float32, float32) {
		r, ok := p.r.tt.Find(label)
		if !ok {
			p.r.t.Fatalf("%q is not on screen: %q", label, p.r.tt.Texts())
		}
		if how == iaRow {
			// The row's own area, clear of the links in its columns.
			return r.X + 16, r.Y + r.H/2
		}
		return r.X + r.W/2, r.Y + r.H/2
	}
	var g iaGest
	p.reset()
	x, y := at()
	p.r.tt.ClickAt(x, y)
	g.click = p.fx()
	p.reset()
	x, y = at()
	p.r.tt.ClickAt(x, y)
	p.r.tt.ClickAt(x, y)
	g.dbl = p.fx()
	// The keys, on one focus: Enter goes last, for it may open a page.
	p.reset()
	if how == iaRow {
		x, y = at()
		p.r.tt.ClickAt(x, y)
		p.r.tt.Frame()
		p.r.tt.Frame()
	} else {
		p.tab(label)
	}
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
		into *string
	}{{ui.Shift, ui.KeyEnter, &g.shiftEnter}, {0, ui.KeySpace, &g.space}, {0, ui.KeyEnter, &g.enter}} {
		p.clear()
		p.r.tt.Key(k.mods, k.key)
		*k.into = p.fx()
	}
	p.reset()
	x, y = at()
	p.r.tt.RightClickAt(x, y)
	g.right = p.fx()
	p.reset()
	return g
}

// The head's Play and Shuffle play the artist's top songs as an artist
// collection: Play from the first song that plays, Shuffle with shuffle on.
// Neither opens anything, and no modifier or right-click does anything.
func TestInteractionArtistHeader(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Radiohead", "Song 1")
	r.tt.Drain()
	pg := iaPageOf(r)
	const play = "play 8 songs start=0 from=artist:657515"
	const shuffle = "play 8 songs from=artist:657515"
	iaWant(t, "Play", pg.gestures("Play Radiohead's top songs", iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
	iaWant(t, "Shuffle", pg.gestures("Shuffle", iaTabStop), iaGest{
		click: iaJoin(shuffle, "shuffle on"), dbl: iaJoin(shuffle, shuffle, "shuffle on"), enter: iaJoin(shuffle, "shuffle on"),
		shiftEnter: "nothing", space: iaJoin(shuffle, "shuffle on"), right: "nothing",
	})
}

// The latest release's slip is a link to its album: a click, a double-click
// (the router keeps one entry) and Enter open it; Space, Shift+Enter and a
// right-click do nothing. Its disc, beside it and not inside, is the Play
// control, which plays the album through the item players and never opens it.
func TestInteractionArtistLatestRelease(t *testing.T) {
	const title = "Hail to the Thief (Live Recordings 2003-2009)"
	r := catalogRig(t, nil)
	pg := iaPageOf(r)
	r.shows("Latest release · May 3, 2016", "Play "+title)
	r.tt.Drain()
	if info, ok := r.tt.Access(title); !ok || info.Role != ui.RoleLink {
		t.Errorf("the slip: role %v (found %v), want a link", info.Role, ok)
	}
	const open = "nav /album/latest"
	iaWant(t, "the slip", pg.gestures(title, iaTabStop), iaGest{
		click: open, dbl: open, enter: open, shiftEnter: "nothing", space: "nothing", right: "nothing",
	})
	const play = "item album:latest"
	iaWant(t, "the slip's disc", pg.gestures("Play "+title, iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
}

// With no page for the album the slip is plain: not a link, not a button, no
// tab stop and no underline under the pointer (the disc plays all the same).
func TestInteractionArtistLatestReleaseWithoutItsAlbumPage(t *testing.T) {
	const title = "Hail to the Thief (Live Recordings 2003-2009)"
	r := catalogRig(t, nil)
	pg := iaPageOf(r)
	r.shows("Latest release · May 3, 2016")
	r.tt.Drain()
	// The slip is built from the route table: no page, no control.
	hasPage := r.h.Env.HasRoute
	r.h.Env.HasRoute = func(path string) bool { return !strings.HasPrefix(path, "/album/") && hasPage(path) }
	r.tt.Frame()
	if info, ok := r.tt.Access(title); ok && (info.Role == ui.RoleButton || info.Role == ui.RoleLink) {
		t.Errorf("the slip: role %v, want no control", info.Role)
	}
	for range 160 {
		if tr := access.Read(t, r.tt.Tester); tr.Focus != nil && tr.Focus.Label == title {
			t.Fatalf("Tab reached the slip of an album with no page")
		}
		r.tt.Key(0, ui.KeyTab)
	}
	iaWant(t, "the slip", pg.gestures(title, iaRow), iaGest{
		click: "nothing", dbl: "nothing", enter: "nothing", shiftEnter: "nothing", space: "nothing", right: "nothing",
	})
}

// The top songs are a track list: a row plays the artist collection from it on
// a double-click or Enter and never on a click, and opens its menu on a
// right-click; Shift+Enter and Space do nothing.
func TestInteractionArtistTopSongs(t *testing.T) {
	r := catalogRig(t, nil)
	r.shows("Song 3")
	r.tt.Drain()
	pg := iaPageOf(r)
	const from = "play 8 songs start=2 from=artist:657515"
	iaWant(t, "Song 3", pg.gestures("Song 3", iaRow), iaGest{
		click: "nothing", dbl: from, enter: from, shiftEnter: "nothing", space: "nothing", right: "menu",
	})
}

// A release in the list is a link to its album: a click opens it (a Cmd or
// Shift click selects it instead), a double-click opens it once, Enter opens
// it; Shift+Enter and Space do nothing. Right-click selects it alone and opens
// its menu. It has no Play control, though the page's latest release, a
// playlist and a similar artist have a disc.
func TestInteractionArtistReleaseRows(t *testing.T) {
	r := catalogRig(t, nil)
	pg := iaPageOf(r)
	pg.list = "essential"
	for _, kind := range []string{services.ItemPlaylist, services.ItemArtist} {
		r.h.Env.Players.Register(kind, func(services.Item) {})
	}
	r.shows("Essential albums by Radiohead", "Kid A", "Play Radiohead Essentials")
	r.tt.Drain()
	if info, ok := r.tt.Access("Kid A"); !ok || info.Role != ui.RoleLink {
		t.Errorf("a release row: role %v (found %v), want a link", info.Role, ok)
	}
	const open = "nav /album/kida"
	iaWant(t, "Kid A", pg.gestures("Kid A", iaTabStop), iaGest{
		click: open, dbl: open, enter: open, shiftEnter: "nothing", space: "nothing", right: "selected 1; menu",
	})
	for _, disc := range []string{
		"Play Hail to the Thief (Live Recordings 2003-2009)", "Play Radiohead Essentials", "Play Thom Yorke",
	} {
		if !r.tt.HasText(disc) {
			t.Errorf("no %q on the page: %q", disc, r.tt.Texts())
		}
	}
	// A release has the Play disc a tile of the same album has: it plays the
	// album through the item players and never opens it.
	for _, album := range []string{"Play Kid A", "Play OK Computer", "Play In Rainbows"} {
		if !r.tt.HasText(album) {
			t.Errorf("%q: a release has no Play control", album)
		}
	}
	const play = "item album:kida"
	iaWant(t, "Kid A's disc", pg.gestures("Play Kid A", iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
}

// The discography's releases are the same links as the lists': a click opens the
// album; a right-click selects it and opens its menu.
func TestInteractionArtistDiscographyReleases(t *testing.T) {
	r := catalogRig(t, nil)
	pg := iaPageOf(r)
	pg.list = "year:2007"
	r.shows("Discography", "In Rainbows")
	r.tt.Drain()
	if info, ok := r.tt.Access("In Rainbows"); !ok || info.Role != ui.RoleLink {
		t.Errorf("a discography release: role %v (found %v), want a link", info.Role, ok)
	}
	const open = "nav /album/ir"
	iaWant(t, "In Rainbows", pg.gestures("In Rainbows", iaTabStop), iaGest{
		click: open, dbl: open, enter: open, shiftEnter: "nothing", space: "nothing", right: "selected 1; menu",
	})
}

// A library artist's head plays the songs the library holds, not the top songs
// the catalogue names: the same Play, labelled with the artist's name.
func TestInteractionArtistLibraryHeader(t *testing.T) {
	r := libraryRig(t, false)
	r.shows("Mia", "Play Mia")
	r.tt.Drain()
	pg := iaPageOf(r)
	pg.path, pg.name = "/artist/r.1", "Mia"
	const play = "play 2 songs start=0 from=artist:r.1"
	iaWant(t, "Play", pg.gestures("Play Mia", iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
}
