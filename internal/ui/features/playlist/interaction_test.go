package playlist

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the sidebar and the
// playlist page do today for each gesture and each kind. The rule being
// reviewed: choosing a result opens it; Play plays; a track list's row plays
// (double-click or Enter); a modifier never does something a visible cue does
// not hint at. Where today's behaviour departs from it, the assertion carries
// a "DEPARTS:" comment, to be found (and flipped) when the behaviour is fixed.

// iaGest is what each gesture on one control did, as iaFx words it.
type iaGest struct {
	click, dbl, enter, shiftEnter, space, right string
}

// iaNav and iaPlay word an effect: a page pushed, a play of the pinned songs.
func iaNav(path string) string { return "nav " + path }

func iaPlay(from string) string { return "play [s1 s2] from=" + from }

// iaJoin is several effects in the order they happened, then shuffle's flag.
func iaJoin(fx ...string) string { return strings.Join(fx, "; ") }

// iaFx waits for what a gesture started (a play runs on a goroutine and posts
// back) and says what it did, "nothing" when it did none.
func (e *sbEnv) iaFx() string {
	for range 12 {
		e.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if p := e.router.Path(); p != "/library/albums" {
		out = append(out, iaNav(p))
	}
	for _, p := range e.play.played() {
		// Shuffle starts from a random song: what plays, not where.
		if i := strings.Index(p, " start="); i >= 0 {
			j := strings.Index(p[i+1:], " ")
			p = p[:i] + p[i+1+j:]
		}
		out = append(out, "play "+p)
	}
	if e.play.shuffled() {
		out = append(out, "shuffle on")
	}
	if len(out) == 0 {
		return "nothing"
	}
	return strings.Join(out, "; ")
}

// iaReset puts the router back where the tests start and forgets the plays.
func (e *sbEnv) iaReset() {
	e.router.Push("/library/albums")
	e.play.mu.Lock()
	e.play.plays, e.play.shuffle = nil, false
	e.play.mu.Unlock()
	e.tt.Frame()
}

// iaTab presses Tab until the control called label has the focus.
func (e *sbEnv) iaTab(label string) {
	e.t.Helper()
	for range 80 {
		if e.tt.Focused(label) {
			return
		}
		e.tt.Key(0, ui.KeyTab)
		e.tt.Frame()
	}
	e.t.Fatalf("Tab never reached %q", label)
}

// iaMenu reports whether a menu of items is open.
func (e *sbEnv) iaMenu() bool {
	return len(access.Read(e.t, e.tt.Tester).Roles(ui.RoleMenuItem)) > 0
}

// iaGestures runs each gesture on the control called label, each from a clean
// start, and says what it did.
func (e *sbEnv) iaGestures(label string) iaGest {
	e.t.Helper()
	r, ok := e.tt.Find(label)
	if !ok {
		e.t.Fatalf("%q is not on screen: %q", label, e.tt.Texts())
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	var g iaGest
	e.iaReset()
	e.tt.ClickAt(x, y)
	g.click = e.iaFx()
	e.iaReset()
	e.tt.ClickAt(x, y)
	e.tt.ClickAt(x, y)
	g.dbl = e.iaFx()
	// The keys, on one focus: Enter goes last, for it may open a page.
	e.iaReset()
	e.iaTab(label)
	for _, k := range []struct {
		mods ui.Modifiers
		key  ui.Key
		into *string
	}{{ui.Shift, ui.KeyEnter, &g.shiftEnter}, {0, ui.KeySpace, &g.space}, {0, ui.KeyEnter, &g.enter}} {
		e.iaReset()
		e.tt.Key(k.mods, k.key)
		*k.into = e.iaFx()
	}
	e.iaReset()
	e.tt.RightClickAt(x, y)
	g.right = e.iaFx()
	if e.iaMenu() {
		g.right = "menu"
		e.tt.Key(0, ui.KeyEscape)
		e.tt.Frame()
	}
	return g
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

// A library playlist row is a link: choosing it opens its page; Shift+Enter
// shuffles it, which the row's cue (the play glyph over its artwork on hover,
// named "Shuffle <name>") hints at; right-click opens no menu (the "More"
// button is the menu's way in).
func TestInteractionSidebarPlaylistRows(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.api.pinTracks["p.own"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	e.api.pinTracks["p.mix1"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	e.shows("Road Trip", "Morning Mix")
	for _, c := range []struct {
		label string
		want  iaGest
	}{
		// The user's own playlist, and one of Apple's personal mixes: both rows
		// are the same link.
		{"Road Trip", iaGest{
			click:      iaNav("/playlist/p.own"),
			dbl:        iaNav("/playlist/p.own"), // the router keeps one entry for the two pushes
			enter:      iaNav("/playlist/p.own"),
			shiftEnter: iaJoin(iaPlay("playlist:p.own"), "shuffle on"),
			space:      "nothing", // a link answers Enter, not Space
			right:      "menu",
		}},
		{"Morning Mix", iaGest{
			click:      iaNav("/playlist/p.mix1"),
			dbl:        iaNav("/playlist/p.mix1"),
			enter:      iaNav("/playlist/p.mix1"),
			shiftEnter: iaJoin(iaPlay("playlist:p.mix1"), "shuffle on"),
			space:      "nothing",
			right:      "menu",
		}},
	} {
		if info, ok := e.tt.Access(c.label); !ok || info.Role != ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a link", c.label, info.Role, ok)
		}
		iaWant(t, c.label, e.iaGestures(c.label), c.want)
	}
}

// The glyph over a row's artwork is the play triangle, but what it does is
// shuffle (its name and tooltip say "Shuffle <name>"): it plays without
// opening the page; a double-click on it shuffles twice.
func TestInteractionSidebarShuffleGlyph(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPlaylistPins("p.pin"))
	e.api.pinTracks["p.own"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	e.api.pinTracks["p.pin"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	e.shows("Road Trip", "p.pin")
	for _, c := range []struct{ row, glyph, from string }{
		{"Road Trip", "Shuffle Road Trip", "playlist:p.own"},
		{"p.pin", "Shuffle p.pin", "playlist:p.pin"},
	} {
		e.iaReset()
		e.hover(c.row)
		if err := e.tt.Click(c.glyph); err != nil {
			t.Fatal(err)
		}
		// DEPARTS: the glyph is the play triangle, but the control shuffles; "Play" never plays from the sidebar's row art, so the cue promises Play and delivers Shuffle.
		if got, want := e.iaFx(), iaJoin(iaPlay(c.from), "shuffle on"); got != want {
			t.Errorf("%s: click on the glyph: %s, want %s", c.row, got, want)
		}

		e.iaReset()
		e.hover(c.row)
		r, _ := e.tt.Find(c.glyph)
		x, y := r.X+r.W/2, r.Y+r.H/2
		e.tt.ClickAt(x, y)
		e.tt.ClickAt(x, y)
		// A double-click on the glyph is two shuffles (each click starts a queue), not one.
		if got, want := e.iaFx(), iaJoin(iaPlay(c.from), iaPlay(c.from), "shuffle on"); got != want {
			t.Errorf("%s: double-click on the glyph: %s, want %s", c.row, got, want)
		}
	}
}

// Pins: a pin set to Open is a link, so choosing it opens its page (an
// artist's does so whatever its action: there is no artist player); one that
// plays or shuffles is a button whose only cue is the words a screen reader
// adds (", plays", ", shuffles"): it looks the same as a row that opens.
func TestInteractionSidebarPins(t *testing.T) {
	pins := []services.Pin{
		{Kind: "playlist", ID: "p.drill", Name: "PL open", Action: "drillIn"},
		{Kind: "playlist", ID: "p.play", Name: "PL play", Action: "play"},
		{Kind: "playlist", ID: "p.shuf", Name: "PL shuffle", Action: "shuffle"},
		{Kind: "album", ID: "l.drill", Name: "AL open", Action: "drillIn"},
		{Kind: "album", ID: "l.play", Name: "AL play", Action: "play"},
		{Kind: "album", ID: "l.shuf", Name: "AL shuffle", Action: "shuffle"},
		{Kind: "song", ID: "i.drill", Name: "SG open", Action: "drillIn", AlbumID: "l.cc"},
		{Kind: "song", ID: "i.play", Name: "SG play", Action: "play", AlbumID: "l.cc"},
		{Kind: "song", ID: "i.shuf", Name: "SG shuffle", Action: "shuffle", AlbumID: "l.cc"},
		{Kind: "song", ID: "i.lone", Name: "SG lone", Action: "play"}, // no album: nothing to open
		{Kind: "artist", ID: "r.drill", Name: "AR open", Action: "drillIn"},
		{Kind: "artist", ID: "r.play", Name: "AR play", Action: "play"},
		{Kind: "artist", ID: "r.shuf", Name: "AR shuffle", Action: "shuffle"},
	}
	e := sbSetup(t, sbLibrary(), pins)
	for _, p := range pins {
		e.api.pinTracks[p.ID] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	}
	e.shows("Pinned", "PL open", "AR shuffle")

	type pinCase struct {
		label string
		role  ui.Role
		want  iaGest
	}
	// A link: opens on a click, a double-click and Enter; Space does nothing.
	link := func(label, path string, shift string) pinCase {
		return pinCase{label, ui.RoleLink, iaGest{
			click: iaNav(path), dbl: iaNav(path), enter: iaNav(path), shiftEnter: shift, space: "nothing",
			right: "menu",
		}}
	}
	// A button: a click plays or shuffles it, a double-click does so twice.
	button := func(label, from string, shuffle bool, shift string) pinCase {
		one := []string{iaPlay(from)}
		two := []string{iaPlay(from), iaPlay(from)}
		if shuffle {
			one, two = append(one, "shuffle on"), append(two, "shuffle on")
		}
		return pinCase{label, ui.RoleButton, iaGest{
			// DEPARTS: a click plays or shuffles with no visible cue: the row looks as the link that opens (only the words a screen reader adds, ", plays" or ", shuffles", tell them apart).
			click:      iaJoin(one...),
			dbl:        iaJoin(two...), // each click is a play: the queue starts twice
			enter:      iaJoin(one...),
			shiftEnter: shift,
			space:      iaJoin(one...), // a button answers Space, a link (above) does not
			right:      "menu",
		}}
	}
	shuffled := func(from string) string { return iaJoin(iaPlay(from), "shuffle on") }
	for _, c := range []pinCase{
		link("PL open", "/playlist/p.drill", shuffled("playlist:p.drill")), // hinted by the hover glyph (a shuffle)
		button("PL play, plays", "playlist:p.play", false, shuffled("playlist:p.play")),
		button("PL shuffle, shuffles", "playlist:p.shuf", true, shuffled("playlist:p.shuf")),
		link("AL open", "/album/l.drill", "nothing"),                    // DEPARTS: Shift+Enter shuffles a playlist pin but does nothing on an album pin, which has no glyph or other cue either
		button("AL play, plays", "album:l.play", false, "nothing"),      // DEPARTS: as above, Shift+Enter does nothing on an album pin
		button("AL shuffle, shuffles", "album:l.shuf", true, "nothing"), // DEPARTS: a pin that shuffles on a click does not shuffle on Shift+Enter, which a playlist pin does
		link("SG open", "/album/l.cc", "nothing"),                       // a song opens its album; DEPARTS: Shift+Enter does nothing on a song pin
		button("SG play, plays", ":i.play", false, "nothing"),           // DEPARTS: Shift+Enter does nothing on a song pin
		button("SG shuffle, shuffles", ":i.shuf", true, "nothing"),      // DEPARTS: Shift+Enter does nothing on a song pin that shuffles on a click
		// A song with no album page to open is a button that plays.
		button("SG lone, plays", ":i.lone", false, "nothing"),
		// An artist pin opens whatever its action says; its menu does not
		// offer Play or Shuffle.
		link("AR open", "/artist/r.drill", "nothing"),
		link("AR play", "/artist/r.play", "nothing"),
		link("AR shuffle", "/artist/r.shuf", "nothing"),
	} {
		info, ok := e.tt.Access(c.label)
		if !ok || info.Role != c.role {
			t.Errorf("%s: role %v (found %v), want %v", c.label, info.Role, ok, c.role)
			continue
		}
		iaWant(t, c.label, e.iaGestures(c.label), c.want)
	}
}

// A pin set to Open whose page Sona does not have does nothing when chosen,
// whatever its kind: it never plays instead, and it is not a link.
func TestInteractionSidebarPinWithoutItsPageDoesNothing(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{
		{Kind: "album", ID: "l.b", Name: "Beta", Action: "drillIn"},
		{Kind: "song", ID: "i.s", Name: "Lone", Action: "drillIn"},
		{Kind: "artist", ID: "r.a", Name: "Nova", Action: "drillIn"},
	})
	e.h.Env.HasRoute = func(path string) bool {
		return !strings.HasPrefix(path, "/album/") && !strings.HasPrefix(path, "/artist/")
	}
	for _, id := range []string{"l.b", "i.s", "r.a"} {
		e.api.pinTracks[id] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	}
	e.shows("Beta", "Lone", "Nova")
	for _, label := range []string{"Beta", "Lone", "Nova"} {
		e.iaReset()
		x, y := e.center(label)
		e.tt.ClickAt(x, y)
		e.tt.ClickAt(x, y)
		if got := e.iaFx(); got != "nothing" {
			t.Errorf("%s: a pin set to Open with no page did %s", label, got)
		}
		if info, ok := e.tt.Access(label); !ok || info.Role == ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a row that is not a link", label, info.Role, ok)
		}
	}
}

// A folder is a disclosure button: choosing it opens or closes it, and it never
// opens a page or plays.
func TestInteractionSidebarFolder(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Running")
	if info, ok := e.tt.Access("Running"); !ok || info.Role != ui.RoleDisclosure {
		t.Fatalf("role %v (found %v), want a disclosure", info.Role, ok)
	}
	open := func() bool { return e.p.prefs.FolderOpen("f.run") }
	r, _ := e.tt.Find("Running")
	x, y := r.X+r.W/2, r.Y+r.H/2

	check := func(gesture string, change bool, do func()) {
		t.Helper()
		before := open()
		e.iaReset()
		do()
		if got := e.iaFx(); got != "nothing" {
			t.Errorf("%s on a folder did %s", gesture, got)
		}
		if (open() != before) != change {
			t.Errorf("%s on a folder: open %v -> %v, changed want %v", gesture, before, open(), change)
		}
	}
	check("click", true, func() { e.tt.ClickAt(x, y) })
	check("double-click", false, func() { e.tt.ClickAt(x, y); e.tt.ClickAt(x, y) }) // open, then shut again
	check("Enter", true, func() { e.iaTab("Running"); e.tt.Key(0, ui.KeyEnter) })
	check("Shift+Enter", false, func() { e.iaTab("Running"); e.tt.Key(ui.Shift, ui.KeyEnter) })
	check("Space", true, func() { e.iaTab("Running"); e.tt.Key(0, ui.KeySpace) })
	check("right-click", false, func() { e.tt.RightClickAt(x, y) })
	if !e.iaMenu() {
		t.Error("right-click on a folder opened no menu")
	}
}

// iaSongs are songs with the pages a row's artist and album link to.
func iaSongs() []services.Song {
	var out []services.Song
	for _, s := range threeSongs() {
		s.ArtistID, s.AlbumID = "ar."+s.ID, "al."+s.ID
		out = append(out, s)
	}
	return out
}

// iaPage drives each gesture on a control of the playlist page from a clean
// start and says what it did: a page pushed, a play of the page's songs.
type iaPage struct {
	r *pageRig
	// path is the page itself, where each gesture starts.
	path string
}

func (p iaPage) fx() string {
	for range 12 {
		p.r.tt.Drain()
		time.Sleep(4 * time.Millisecond)
	}
	var out []string
	if path := p.r.router.Path(); path != p.path {
		out = append(out, iaNav(path))
	}
	shuffled := p.r.play.shuffled()
	for _, s := range p.r.play.played() {
		// A shuffle starts from a random song: what plays, not where.
		if i := strings.Index(s, " start="); i >= 0 && shuffled {
			j := strings.Index(s[i+1:], " ")
			s = s[:i] + s[i+1+j:]
		}
		out = append(out, "play "+s)
	}
	if shuffled {
		out = append(out, "shuffle on")
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
	left := p.r.router.Path() != p.path
	p.r.router.Push(p.path)
	p.clear()
	if left {
		p.r.tt.Until("the page", func() bool { return p.r.router.Path() == p.path && p.r.tt.HasText("Song A") })
		p.top()
	}
	p.r.tt.Frame()
}

// tab presses Tab until the control called label has the focus, which the
// accessibility tree names (a link's text is not its control).
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

func (p iaPage) menu() bool {
	return len(access.Read(p.r.t, p.r.tt.Tester).Roles(ui.RoleMenuItem)) > 0
}

// How a control takes the keyboard.
const (
	iaTabStop = iota // Tab reaches it
	iaRow            // a track row: one stop of its list, focused by a click
	iaPointer        // a link inside a row: pointer only, no stop of its own
)

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
	if p.menu() {
		g.right = "menu"
		p.r.tt.Key(0, ui.KeyEscape)
		p.r.tt.Frame()
	}
	return g
}

// The page's Play and Shuffle buttons play the playlist as the page lists it:
// they never open anything.
func TestInteractionPlaylistPageHeader(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", iaSongs()...))
	r.shows("Song A", "Play Road Trip")
	pg := iaPage{r, "/playlist/p.1"}
	const play = "play [a b c] start=0 from=playlist:p.1"
	const shuffle = "play [a b c] from=playlist:p.1"
	iaWant(t, "Play", pg.gestures("Play Road Trip", iaTabStop), iaGest{
		click: play, dbl: iaJoin(play, play), enter: play, shiftEnter: "nothing", space: play, right: "nothing",
	})
	iaWant(t, "Shuffle", pg.gestures("Shuffle", iaTabStop), iaGest{
		click: iaJoin(shuffle, "shuffle on"), dbl: iaJoin(shuffle, shuffle, "shuffle on"), enter: iaJoin(shuffle, "shuffle on"),
		shiftEnter: "nothing", space: iaJoin(shuffle, "shuffle on"), right: "nothing",
	})
}

// The page's track list: a row plays the playlist from it on a double-click or
// Enter and never on a click, and opens its menu on a right-click; the artist
// and album names in it are links that open their pages on a click, with no
// Tab stop of their own.
func TestInteractionPlaylistPageRows(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", iaSongs()...))
	r.shows("Song B", "Artist B", "Album B")
	pg := iaPage{r, "/playlist/p.1"}
	const from = "play [a b c] start=1 from=playlist:p.1"
	iaWant(t, "Song B", pg.gestures("Song B", iaRow), iaGest{
		click: "nothing", dbl: from, enter: from, shiftEnter: "nothing", space: "nothing", right: "menu",
	})
	for _, c := range []struct{ name, path string }{{"Artist B", "/artist/ar.b"}, {"Album B", "/album/al.b"}} {
		if info, ok := r.tt.Access(c.name); !ok || info.Role != ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a link", c.name, info.Role, ok)
		}
		iaWant(t, c.name, pg.gestures(c.name, iaPointer), iaGest{
			click: iaNav(c.path), dbl: iaNav(c.path), enter: "n/a", shiftEnter: "n/a", space: "n/a",
			// DEPARTS: right-click on the name opens no menu, though the rest of its row opens the song's.
			right: "nothing",
		})
	}
}

// The byline names who made the playlist: a link to the curator's page that
// opens it, where Sona has the page.
func TestInteractionPlaylistPageByline(t *testing.T) {
	d := catalogDetail("pl.1", "Indie Mix", iaSongs()...)
	d.Curator = services.PlaylistCurator{ID: "ac.1", Kind: "apple-curator", Name: "Apple Music Indie"}
	r := newPageRig(t, "pl.1", d)
	r.shows("Apple Music Indie")
	if info, ok := r.tt.Access("Apple Music Indie"); !ok || info.Role != ui.RoleLink {
		t.Errorf("role %v (found %v), want a link", info.Role, ok)
	}
	const to = "nav /curator/ac.1"
	iaWant(t, "the curator", iaPage{r, "/playlist/pl.1"}.gestures("Apple Music Indie", iaTabStop), iaGest{
		click: to, dbl: to, enter: to, shiftEnter: "nothing", space: "nothing", right: "nothing",
	})
}
