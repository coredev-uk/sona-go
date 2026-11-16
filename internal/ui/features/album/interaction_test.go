package album

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the album page does
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
	iaPointer        // a link inside a row: pointer only, no stop of its own
)

// iaPage drives the gestures of an album page from a clean start and says what
// each did: a page pushed, a play of the album's songs, a menu opened.
type iaPage struct {
	r *rig
	// path is the page itself, where each gesture starts.
	path string
}

func (p iaPage) fx() string {
	for range 12 {
		p.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if path := p.r.path(); path != p.path {
		out = append(out, "nav "+path)
	}
	p.r.queue.mu.Lock()
	for _, pl := range p.r.queue.plays {
		out = append(out, "play "+iaPlay(pl, p.r.queue.shuffle))
	}
	if p.r.queue.shuffle {
		out = append(out, "shuffle on")
	}
	p.r.queue.mu.Unlock()
	if n := len(access.Read(p.r.t, p.r.tt.Tester).Roles(ui.RoleMenuItem)); n > 0 {
		out = append(out, "menu")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// iaPlay words a play: the songs, where it started (a shuffle starts from a
// random one: not said) and where from.
func iaPlay(pl played, shuffled bool) string {
	start := ""
	if !shuffled {
		start = " start=" + string(rune('0'+pl.start))
	}
	return "[" + strings.Join(pl.ids, " ") + "]" + start + " from=" + pl.from.Kind + ":" + pl.from.ID
}

// clear forgets what a gesture did, without leaving the page.
func (p iaPage) clear() {
	p.r.queue.mu.Lock()
	p.r.queue.plays, p.r.queue.shuffle = nil, false
	p.r.queue.mu.Unlock()
}

// top scrolls the page to its top (Tab scrolls to the control it reaches).
func (p iaPage) top() {
	p.r.tt.Scroll(600, 400, 0, -10000)
	p.r.tt.Settle()
}

// reset closes what a gesture opened and forgets what it did. A page that was
// opened is built again: wait for its songs, at its top.
func (p iaPage) reset() {
	p.r.t.Helper()
	p.r.tt.Key(0, ui.KeyEscape)
	left := p.r.path() != p.path
	p.r.router.Push(p.path)
	p.clear()
	if left {
		p.r.tt.Until("the page", func() bool { return p.r.path() == p.path && p.r.tt.HasText("Evening Bus") })
		p.top()
	}
	p.r.tt.Frame()
}

// tab presses Tab until the control called label has the focus, which the
// accessibility tree names.
func (p iaPage) tab(label string) {
	p.r.t.Helper()
	for range 120 {
		if tr := access.Read(p.r.t, p.r.tt.Tester); tr.Focus != nil && tr.Focus.Label == label {
			return
		}
		p.r.tt.Key(0, ui.KeyTab)
	}
	p.r.t.Fatalf("Tab never reached %q", label)
}

// gestures runs each gesture on the control called label. A pointer-only
// control has no keys to press ("n/a").
func (p iaPage) gestures(label string, how int) iaGest {
	p.r.t.Helper()
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
	g := iaGest{enter: "n/a", shiftEnter: "n/a", space: "n/a"}
	p.reset()
	x, y := at()
	p.r.tt.ClickAt(x, y)
	g.click = p.fx()
	p.reset()
	x, y = at()
	p.r.tt.ClickAt(x, y)
	p.r.tt.ClickAt(x, y)
	g.dbl = p.fx()
	if how != iaPointer {
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
		p.top()
	}
	p.reset()
	x, y = at()
	p.r.tt.RightClickAt(x, y)
	g.right = p.fx()
	p.reset()
	return g
}

// The head's Play and Shuffle buttons play the album: Play from its first song,
// Shuffle from a random one with shuffle on. Neither opens anything, and no
// modifier or right-click does anything.
func TestInteractionAlbumHeader(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Play Salt Flats", "Shuffle")
	pg := iaPage{r, "/album/100"}
	const play = "play [t1 t2 t3 t4 t5] start=0 from=album:100"
	const shuffle = "play [t1 t2 t3 t4 t5] from=album:100"
	iaWant(t, "Play", pg.gestures("Play Salt Flats", iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
	iaWant(t, "Shuffle", pg.gestures("Shuffle", iaTabStop), iaGest{
		click: iaJoin(shuffle, "shuffle on"), dbl: iaJoin(shuffle, shuffle, "shuffle on"), enter: iaJoin(shuffle, "shuffle on"),
		shiftEnter: "nothing", space: iaJoin(shuffle, "shuffle on"), right: "nothing",
	})
}

// An album not out yet plays what is out already from a disc named for it
// ("Play the 2 songs out now"), and only that. The disc acts as Play does on
// an album that is out; there is no Shuffle beside it.
func TestInteractionAlbumNotOutYet(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	release := r.now.Add(5*time.Hour + 12*time.Minute)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Prerelease, p.Album.ReleaseAt, p.Album.ReleaseDate = true, release.UnixMilli(), release.Format("2006-01-02")
		for i := 2; i < 5; i++ { // three songs are not out yet
			p.Tracks[i].Playable = false
		}
	}))
	r.shows("Play the 2 songs out now")
	if r.tt.HasText("Shuffle") {
		t.Error("an album not out yet offers Shuffle")
	}
	const play = "play [t1 t2] start=0 from=album:100"
	iaWant(t, "Play the 2 songs out now", iaPage{r, "/album/100"}.gestures("Play the 2 songs out now", iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
}

// The track list: a row plays the album from it on a double-click or Enter and
// never on a click, and opens its menu on a right-click; Shift+Enter and Space
// do nothing.
func TestInteractionAlbumRows(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) { p.Tracks[2].Playable = false }))
	r.shows("Evening Bus", "Glass Houses")
	pg := iaPage{r, "/album/100"}
	// Song 4 is the third playable one; the greyed song is left out of the queue.
	const from = "play [t1 t2 t4 t5] start=2 from=album:100"
	iaWant(t, "Evening Bus", pg.gestures("Evening Bus", iaRow), iaGest{
		click: "nothing", dbl: from, enter: from, shiftEnter: "nothing", space: "nothing", right: "menu",
	})
	// A song that cannot play: the row is greyed, and choosing it plays nothing.
	iaWant(t, "Glass Houses (unavailable)", pg.gestures("Glass Houses", iaRow), iaGest{
		click: "nothing", dbl: "nothing", enter: "nothing", shiftEnter: "nothing", space: "nothing", right: "menu",
	})
}

// The byline names the artist: a link to their page that opens it on a click, a
// double-click and Enter, where Sona has the page.
func TestInteractionAlbumByline(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats())
	r.shows("Salt Flats", "Mia Vale")
	if info, ok := r.tt.Access("Mia Vale"); !ok || info.Role != ui.RoleLink {
		t.Errorf("the byline: role %v (found %v), want a link", info.Role, ok)
	}
	const to = "nav /artist/a1"
	iaWant(t, "the byline", iaPage{r, "/album/100"}.gestures("Mia Vale", iaTabStop), iaGest{
		click: to, dbl: to, enter: to, shiftEnter: "nothing", space: "nothing", right: "nothing",
	})
}

// On a compilation the Artist column names each song's artist as a link to
// their page: a click opens it, and it has no Tab stop of its own (the row is
// the list's stop).
func TestInteractionAlbumCompilationArtistColumn(t *testing.T) {
	r := newRig(t, "/album/100", 1132, 1000)
	r.addCatalog(saltFlats(func(p *services.CatalogAlbum) {
		p.Album.Artist = "Various Artists"
		p.Tracks[0].Artist, p.Tracks[1].Artist, p.Tracks[2].Artist = "Ana", "Bo", "Cy"
	}))
	r.shows("Salt Flats", "Ana", "Bo", "Cy")
	if info, ok := r.tt.Access("Ana"); !ok || info.Role != ui.RoleLink {
		t.Errorf("an artist in the column: role %v (found %v), want a link", info.Role, ok)
	}
	const to = "nav /artist/a1"
	iaWant(t, "Ana", iaPage{r, "/album/100"}.gestures("Ana", iaPointer), iaGest{
		click: to, dbl: to, enter: "n/a", shiftEnter: "n/a", space: "n/a",
		right: "menu",
	})
}
