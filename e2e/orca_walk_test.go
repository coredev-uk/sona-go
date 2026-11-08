//go:build e2e

package e2e

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// TestOrcaWalk is a person's walk with Orca, by keyboard, through the
// sidebar, the account menu and dialog, the Albums page, an album, the queue,
// Now Playing and the player bar's Seek slider, with what Orca says for each
// key checked against docs/accessibility.md (E2E_ORCA=1, `orca-walk`).
//
// Two things are checked on every key (orca.press): nothing is said twice in
// a row (a name and the text it is made of), and no key is answered with a
// flood (a control whose value Orca reads as often as it changes). The rest
// is what each surface must say: its roles, names and states, once; and where
// the focus goes when a menu, dialog or panel closes.
func TestOrcaWalk(t *testing.T) {
	_ = orcaBin(t)
	a := startApp(t)
	c := a.client
	if _, err := c.Wait(startTimeout, "the Albums page", func(tr *automation.Tree) bool { return tr.HasText("24 albums") }); err != nil {
		t.Fatal(err)
	}
	o := startOrca(t)
	defer o.transcript("orca-walk.md")
	albums := fixture.Albums()
	big := bigAlbum()

	// The sidebar: one stop per link, each a link, the page that is open
	// "Current page", the playlists a list of three.
	const side = "Sidebar"
	n := len(o.steps)
	sp := o.press(side, "Tab")
	o.want(side, "Tab", sp, "Main", "For you", "link")
	o.tabTo(side, "Tab", "Settings", 30)
	o.wantOrder(side, n, "For you", "Discover", "Charts", "Radio", "Search", "Concerts",
		"Recently added", "Artists", "Albums", "Songs", "Music videos", "Favourite songs", "History", "Playlists",
		"New playlist or folder", "Playlists", "Focus", "Road trip", "Sunday morning", "Settings")
	o.wantOnce(side, "Tab", o.heard(n), "Current page")
	o.want(side, "Tab", o.heard(n), "push button menu", "List with 3 items")

	// The top bar: the account button is named once, and its description is
	// where the account stands, not its name again.
	const top = "Top bar"
	sp = o.press(top, "Tab")
	o.want(top, "Tab", sp, "search", "Search", "editable combo box")
	sp = o.press(top, "Tab")
	o.want(top, "Tab", sp, "Account", "button", "Not signed in")
	o.wantOnce(top, "Tab", sp, "Account")

	// The account menu opens on its first item and gives the focus back to
	// the button; the dialog it opens does too.
	const menu = "Account menu"
	sp = o.press(menu, "Return")
	o.want(menu, "Return", sp, "menu", "Sign in")
	o.wantFocus(menu, "Return", "menu item: 'Sign in'")
	o.press(menu, "Down")
	o.press(menu, "Up")
	sp = o.press(menu, "Escape")
	o.want(menu, "Escape", sp, "Account", "button")
	o.wantFocus(menu, "Escape", "button: 'Account'")
	o.press(menu, "Return")
	const dialog = "Account dialog"
	sp = o.press(dialog, "Return")
	o.want(dialog, "Return", sp, "dialog", "Apple Account", "entry")
	o.wantFocus(dialog, "Return", "entry: 'Apple Account'")
	o.tabTo(dialog, "Tab", "Close", 8)
	sp = o.press(dialog, "Escape")
	o.want(dialog, "Escape", sp, "Account", "button")
	o.wantFocus(dialog, "Escape", "button: 'Account'")

	// A library page: its heading's panel, the filter's toggles and their
	// states, Sort, then the tiles, one stop each, with the arrows moving
	// between them and Shift+F10 opening a tile's menu.
	const page = "Albums page"
	sp = o.press(page, "Tab")
	o.want(page, "Tab", sp, "Albums", "panel", "search", "Filter albums", "Album type", "All", "toggle button pressed")
	sp = o.tabTo(page, "Tab", "Sort", 4)
	o.want(page, "Tab", sp, "combo box")
	first := albums[len(albums)-1]
	sp = o.press(page, "Tab")
	o.want(page, "Tab", sp, first.Name, "link", first.Artist)
	o.wantOnce(page, "Tab", sp, first.Name)
	sp = o.press(page, "Right")
	o.want(page, "Right", sp, albums[len(albums)-2].Name, "link")
	o.press(page, "Down")
	tile := focusName(o.focus)
	sp = o.press(page, "shift+F10")
	o.want(page, "shift+F10", sp, "More for "+tile, "menu")
	o.press(page, "Down")
	sp = o.press(page, "Escape")
	o.want(page, "Escape", sp, tile, "link")
	o.wantFocus(page, "Escape", "link: '"+tile+"'")

	// An album: its page is announced, an artist link is read once (not as a
	// text with a link in it), the buttons say what they do, the song list is
	// one stop with the arrows inside, and Enter plays and announces once.
	const album = "Album page"
	o.tabTo(page, "Tab", big.Name, 30)
	sp = o.press(album, "Return")
	o.want(album, "Return", sp, "Album")
	o.wantFocus(album, "Return", "panel: 'Album'")
	sp = o.press(album, "Tab")
	o.want(album, "Tab", sp, big.Artist, "link")
	o.wantOnce(album, "Tab", sp, big.Artist)
	sp = o.press(album, "Tab")
	o.want(album, "Tab", sp, "Play "+big.Name, "button")
	n = len(o.steps)
	o.tabTo(album, "Tab", big.Song(0), 12)
	o.want(album, "Tab", o.heard(n), "Favourite "+big.Name, "toggle button not pressed", "More for "+big.Name, "push button menu")
	sp = o.press(album, "Down")
	o.want(album, "Down", sp, big.Song(1))
	o.press(album, "Up")
	sp = o.press(album, "Return")
	o.wantOnce(album, "Return", sp, "Now playing: "+big.Song(0)+" by "+big.Artist)

	// The queue beside the page. Its shortcut says nothing (a known gap), the
	// list is one stop, each move is read once with the song's new place,
	// removing moves the focus to the row that takes its place, and Escape
	// returns it to the Queue toggle.
	const queue = "Queue"
	o.press(queue, "q")
	n = len(o.steps)
	o.tabTo(queue, "Tab", "List with", 8)
	o.want(queue, "Tab", o.heard(n), "Queue", "List with 3 items")
	sp = o.press(queue, "Down")
	o.want(queue, "Down", sp, big.Song(1))
	sp = o.press(queue, "alt+Down")
	o.wantOnce(queue, "alt+Down", sp, "moved to position")
	o.want(queue, "alt+Down", sp, big.Song(1)+": moved to position 2 of 2")
	sp = o.press(queue, "alt+Up")
	o.wantOnce(queue, "alt+Up", sp, "moved to position")
	o.want(queue, "alt+Up", sp, big.Song(1)+": moved to position 1 of 2")
	sp = o.press(queue, "Delete")
	if !strings.HasPrefix(o.focus, "list item: ") {
		t.Errorf("%s, Delete: the focus is on [%s], not the row that took the removed one's place", queue, o.focus)
	}
	if slices.ContainsFunc(sp, func(u string) bool { return strings.Contains(u, "moved") }) {
		t.Errorf("%s, Delete: removing says it moved something: %q", queue, sp)
	}
	sp = o.press(queue, "Escape")
	o.want(queue, "Escape", sp, "Queue", "toggle button", "not pressed")
	o.wantFocus(queue, "Escape", "toggle button: 'Queue'")

	// Now Playing: the panes are tabs, one stop, the arrows move and show;
	// Escape first hides the tooltip of the focused Close button (WCAG 1.4.13),
	// then closes the view.
	const np = "Now Playing"
	sp = o.press(np, "n")
	o.want(np, "n", sp, "Lyrics", "page tab")
	o.wantFocus(np, "n", "page tab: 'Lyrics'")
	sp = o.press(np, "Right")
	o.want(np, "Right", sp, "Up next", "page tab")
	sp = o.press(np, "Left")
	o.want(np, "Left", sp, "Lyrics", "page tab")
	sp = o.press(np, "Tab")
	o.want(np, "Tab", sp, "Close now playing", "button")
	o.press(np, "Escape")
	o.press(np, "Escape")
	if _, err := c.Wait(10*time.Second, "Now Playing to close", func(tr *automation.Tree) bool {
		_, open := tr.Find("button", "Close now playing (Escape)")
		return !open
	}); err != nil {
		t.Error(err)
	}
	if o.focus != "DEAD" {
		// Known gap (docs/accessibility.md): a view a key opened has no opener.
		t.Logf("the focus after Escape is [%s]: a key-opened Now Playing now gives it back; update the docs' Known gaps", o.focus)
	}

	// The player bar, from its end: the Volume slider says its value once
	// (the number and the percentage, no third "80%"), and a focused Seek
	// slider is read once and not again as the song plays on.
	const bar = "Player bar"
	n = len(o.steps)
	sp = o.tabTo(bar, "shift+Tab", "Seek", 12)
	o.want(bar, "shift+Tab", sp, "slider", "of 3:00")
	o.want(bar, "shift+Tab", o.heard(n), "Volume", "horizontal slider")
	if slices.Contains(o.heard(n), "80%") {
		t.Errorf("%s: Volume is read as a value, a percentage and a third time as 80%%: %q", bar, o.heard(n))
	}
	o.wantSilence(bar, 4*time.Second)
	o.press(bar, "Tab")
	sp = o.press(bar, "ctrl+Right")
	o.wantOnce(bar, "ctrl+Right", sp, "Now playing: "+big.Song(2)+" by "+big.Artist)
}

// TestOrcaSetup is the first-run setup with Orca: the step's title takes the
// focus and is read (a focused heading would be read as "heading 2" only),
// the controls follow, and the confirmation Escape asks for is an alert.
func TestOrcaSetup(t *testing.T) {
	_ = orcaBin(t)
	a := startAppWith(t, "-first-run")
	if _, err := a.client.Wait(startTimeout, "the first-run setup", func(tr *automation.Tree) bool {
		return tr.HasText("Set up Sona") && tr.HasText("Welcome to Sona")
	}); err != nil {
		t.Fatal(err)
	}
	o := startOrca(t)
	defer o.transcript("orca-setup.md")

	const welcome = "Setup welcome"
	sp := o.press(welcome, "Tab")
	o.want(welcome, "Tab", sp, "Welcome to Sona", "dialog", "Language", "combo box")
	sp = o.press(welcome, "Tab")
	o.want(welcome, "Tab", sp, "Get started", "button")
	sp = o.press(welcome, "Return")
	o.want(welcome, "Return", sp, "Apple Music")
	o.wantFocus(welcome, "Return", "panel: 'Apple Music'")
	if slices.Contains(sp, "heading 2") {
		t.Errorf("%s: the step's title is read as %q only: %q", welcome, "heading 2", sp)
	}

	const step = "Setup Apple Music step"
	sp = o.tabTo(step, "Tab", "Apple Account", 4)
	o.want(step, "Tab", sp, "entry")
	sp = o.press(step, "Tab")
	o.want(step, "Tab", sp, "Password", "password text")
	o.tabTo(step, "Tab", "Skip setup", 8)

	// Escape from the title asks before leaving: an alert on its button.
	const confirm = "Setup confirmation"
	o.tabTo(step, "Tab", "Apple Music", 4)
	sp = o.press(confirm, "Escape")
	o.want(confirm, "Escape", sp, "alert", "Skip the rest of the setup?", "Skip setup", "button")
	sp = o.press(confirm, "Tab")
	o.want(confirm, "Tab", sp, "Cancel", "button")
	o.press(confirm, "Escape")
	// Known gap (docs/accessibility.md): the focus returns to the setup's
	// first control, not to what asked.
	t.Logf("the focus after cancelling the confirmation is [%s]", o.focus)
}

// focusName is the name of the element in a focus "role: 'name'".
func focusName(focus string) string {
	_, name, _ := strings.Cut(focus, "'")
	return strings.TrimSuffix(name, "'")
}

// bigAlbum is a fixture album with three songs: two stay in the queue after
// the first.
func bigAlbum() fixture.Album {
	for _, a := range fixture.Albums() {
		if a.SongCount() >= 3 {
			return a
		}
	}
	panic("no fixture album with three songs")
}
