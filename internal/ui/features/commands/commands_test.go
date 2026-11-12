package commands

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// rig is the feature over a full Env (the fixture library), a router and a
// view that does what the shell's dispatcher does for its two chords.
type rig struct {
	t      *testing.T
	m      *commands
	mod    module.Module
	router *ui.Router
	tt     *uitest.Session
	h      *uitest.Harness
	routes []string
	ran    []string
	extra  []module.BoundShortcut
}

func (r *rig) hasRoute(path string) bool {
	return slices.ContainsFunc(r.routes, func(p string) bool { return strings.HasPrefix(path, p) })
}

func newRig(t *testing.T, opts ...uitest.Option) *rig {
	t.Helper()
	opts = append(opts, uitest.Albums(fixture.Album{Name: "Zebra Crossing", Artist: "Mia", Titles: []string{"Quartz Lantern"}}))
	h := uitest.New(t, opts...)
	env := h.Env
	r := &rig{t: t, h: h, routes: []string{"/library/albums", "/album/", "/artist/", "/playlist/", "/search", "/settings"}}
	r.router = ui.NewRouter("/library/albums")
	env.Nav = r.router
	env.HasRoute = r.hasRoute
	r.mod = New(env)
	r.m = newCommands(env)
	r.m.lib = services.Service[services.SearchService](env.Core)
	// The module's own shortcuts run on the rig's state, so the test view
	// and the entries share it.
	r.mod.Shortcuts[0].Run = r.m.togglePalette
	r.mod.Shortcuts[1].Run = r.m.toggleHelp
	run := func(id string) func() { return func() { r.ran = append(r.ran, id) } }
	r.extra = []module.BoundShortcut{
		{Shortcut: module.Shortcut{
			ID: "player.toggle", Label: func() string { return "Play or pause" }, Group: "playback", Icon: "pause",
			Keywords: func() string { return "resume stop" }, Run: run("toggle"),
		}, Keys: []string{"Space"}},
		{Shortcut: module.Shortcut{
			ID: "player.next", Label: func() string { return "Next track" }, Group: "playback", Icon: "skip-forward",
			Keywords: func() string { return "skip forward" }, Run: run("next"),
		}, Keys: []string{"Ctrl+ArrowRight"}},
		{Shortcut: module.Shortcut{
			ID: "player.mute", Label: func() string { return "Mute or unmute" }, Group: "playback",
			When: func() bool { return false }, Run: run("mute"),
		}, Keys: []string{"M"}},
		{Shortcut: module.Shortcut{
			ID: "player.previous", Label: func() string { return "Previous track" }, Group: "playback",
			Unavailable: func() string { return "Live" }, Run: run("previous"),
		}, Keys: []string{"Ctrl+ArrowLeft"}},
		{Shortcut: module.Shortcut{
			ID: "nav.library.albums", Label: func() string { return "Library · Albums" }, Group: "pages", Icon: "disc-3",
			Run: func() { r.router.Push("/library/albums") },
		}, Keys: []string{"Alt+1"}},
		{Shortcut: module.Shortcut{ID: "shell.sidebar", Label: func() string { return "Show or hide the sidebar" }, Group: "view", Run: run("sidebar")}},
	}
	env.Shortcuts = func() []module.BoundShortcut {
		out := slices.Clone(r.extra)
		keys := [][]string{{"Ctrl+K"}, {"?"}}
		for i, s := range r.mod.Shortcuts {
			out = append(out, module.BoundShortcut{Shortcut: s, Keys: keys[i]})
		}
		return out
	}
	r.tt = h.Tester(t, func(c *ui.Context) {
		theme.Apply(c)
		if c.Shortcut(ui.Ctrl, ui.KeyK) {
			r.mod.Shortcuts[0].Run()
		}
		if c.Shortcut(ui.Shift, ui.KeySlash) {
			r.mod.Shortcuts[1].Run()
		}
		ui.Column(c).Padding(20).AlignItems(ui.Start).Children(func() {
			kit.Button(c.Key("opener"), "Opener", kit.ButtonOpts{})
		})
		r.m.layer(c)
	}, 1132, 800)
	r.tt.Frame()
	return r
}

// focusOpener puts the focus on the page's button, where the palette
// should return it to.
func (r *rig) focusOpener() {
	r.t.Helper()
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Opener") {
		r.t.Fatal("the opener did not take the focus")
	}
}

func (r *rig) settle() { r.tt.Advance(400 * time.Millisecond) }

func TestCtrlKOpensThePaletteWithEveryAvailableCommandByGroup(t *testing.T) {
	r := newRig(t)
	r.focusOpener()
	r.tt.Key(ui.Ctrl, ui.KeyK)
	if !r.m.pal.IsOpen() {
		t.Fatal("Ctrl+K did not open the palette")
	}
	if !r.tt.Focused("Find a page, action or music in your library") {
		t.Error("the field does not have the focus")
	}
	for _, s := range []string{
		"Playback", "Play or pause", "Next track", "View", "Show or hide the sidebar", "Pages", "Library · Albums",
		"General", "Keyboard shortcuts", "Move", "Run", "Close",
	} {
		if !r.tt.HasText(s) {
			t.Errorf("the palette does not show %q; texts %q", s, r.tt.Texts())
		}
	}
	for _, s := range []string{
		"Open the command palette", // palette:false
		"Mute or unmute",           // its When is false
		"Previous track",           // unavailable
		"Navigation",               // an empty group is left out
		"Commands", "In your library", "Apple Music",
	} {
		if r.tt.HasText(s) {
			t.Errorf("the palette shows %q", s)
		}
	}
}

func TestPaletteTypingRanksCommandsAndOffersAppleMusic(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("next")
	for _, s := range []string{"Commands", "Next track", "Apple Music", "Search Apple Music for “next”"} {
		if !r.tt.HasText(s) {
			t.Errorf("typing next: %q is missing; texts %q", s, r.tt.Texts())
		}
	}
	if r.tt.HasText("Play or pause") {
		t.Error("a command that does not match is listed")
	}
	// Keywords match last: "skip" finds Next track.
	for range 4 {
		r.tt.Key(0, ui.KeyBackspace)
	}
	r.tt.Type("skip")
	if !r.tt.HasText("Next track") || r.tt.HasText("Play or pause") {
		t.Errorf("skip: %q", r.tt.Texts())
	}
}

func TestPaletteEnterRunsTheCommandAfterClosingWithFocusBack(t *testing.T) {
	r := newRig(t)
	r.focusOpener()
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("next")
	r.tt.Key(0, ui.KeyEnter)
	r.settle()
	if r.m.pal.IsOpen() {
		t.Fatal("Enter left the palette open")
	}
	if !slices.Equal(r.ran, []string{"next"}) {
		t.Errorf("ran %v, want [next]", r.ran)
	}
	if !r.tt.Focused("Opener") {
		t.Error("focus did not return to where it was")
	}
}

func TestPaletteEscapeClosesAndReturnsFocus(t *testing.T) {
	r := newRig(t)
	r.focusOpener()
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	if r.m.pal.IsOpen() || len(r.ran) != 0 {
		t.Fatalf("Escape: open %v, ran %v", r.m.pal.IsOpen(), r.ran)
	}
	if !r.tt.Focused("Opener") {
		t.Error("focus did not return to the opener")
	}
}

func TestPalettesOwnChordClosesIt(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.settle()
	if r.m.pal.IsOpen() {
		t.Error("Ctrl+K did not close the open palette")
	}
}

func TestPaletteArrowsMoveAndWrap(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Key(0, ui.KeyUp) // wraps from the first to the last
	r.tt.Key(0, ui.KeyEnter)
	r.settle()
	// The last command of the last group (General): Keyboard shortcuts.
	if !r.m.help.open {
		t.Errorf("Up from the first did not wrap to the last command; ran %v", r.ran)
	}
}

func TestPaletteFindsLibraryItemsAfterAPauseAndOpensThem(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("zebra")
	if r.tt.HasText("In your library") {
		t.Error("the library was asked before the typing paused")
	}
	r.tt.Advance(libraryDelay + 10*time.Millisecond)
	r.tt.Until("the library section", r.tt.Shows("In your library", "Zebra Crossing", "Album · Mia"))
	if !r.tt.HasText("Search Apple Music for “zebra”") {
		t.Error("the Apple Music entry is missing under the library's")
	}
	r.tt.MustClick("Zebra Crossing")
	r.settle()
	if got := r.router.Path(); !strings.HasPrefix(got, "/album/") {
		t.Errorf("picking the album went to %q", got)
	}
}

func TestPalettePlaysASongPickedFromTheLibrary(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("quartz")
	r.tt.Advance(libraryDelay + 10*time.Millisecond)
	r.tt.Until("the song", r.tt.Shows("Quartz Lantern", "Song · Mia"))
	r.tt.MustClick("Quartz Lantern")
	r.settle()
	r.tt.Until("the song queued through env.Queue", func() bool {
		r.h.Main.Drain()
		q := r.h.Env.Play.Queue()
		return len(q.Items) == 1 && q.Items[0].Name == "Quartz Lantern"
	})
	if got := r.router.Path(); got != "/library/albums" {
		t.Errorf("playing a song navigated to %q", got)
	}
}

func TestPaletteLeavesOutWhatNoPageCanShow(t *testing.T) {
	r := newRig(t)
	r.routes = []string{"/library/albums"} // no album page, no search
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("zebra")
	r.tt.Advance(libraryDelay + 10*time.Millisecond)
	r.settle()
	if r.tt.HasText("Zebra Crossing") || r.tt.HasText("Apple Music") {
		t.Errorf("entries for pages that do not exist: %q", r.tt.Texts())
	}
}

func TestPaletteAppleMusicEntrySearches(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.Type("kind of blue")
	r.tt.MustClick("Search Apple Music for “kind of blue”")
	r.settle()
	if got := r.router.Location(); got != "/search?q=kind+of+blue" {
		t.Errorf("the search went to %q", got)
	}
}

func TestPaletteMouseMoveMakesAnOptionActiveAndClickPicksIt(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.tt.MustClick("Play or pause")
	r.settle()
	if !slices.Equal(r.ran, []string{"toggle"}) {
		t.Errorf("ran %v", r.ran)
	}
	if r.m.pal.IsOpen() {
		t.Error("a click left the palette open")
	}
}

func TestQuestionMarkOpensTheHelpListingGroups(t *testing.T) {
	r := newRig(t)
	r.focusOpener()
	r.tt.Key(ui.Shift, ui.KeySlash)
	if !r.m.help.open {
		t.Fatal("? did not open the shortcut help")
	}
	for _, s := range []string{
		"Keyboard shortcuts", "Change any of them in", "Settings", "Playback", "View", "Pages", "General",
		"Play or pause", "Next track", "Show or hide the sidebar", "Library · Albums", "Open the command palette",
		"Space", "Ctrl",
	} {
		if !r.tt.HasText(s) {
			t.Errorf("the help does not show %q; texts %q", s, r.tt.Texts())
		}
	}
	// It lists every shortcut, available or not, and omits an empty group.
	if !r.tt.HasText("Mute or unmute") || !r.tt.HasText("Previous track") {
		t.Error("the help leaves out shortcuts that are not available now")
	}
	if r.tt.HasText("Navigation") {
		t.Error("an empty group has a heading")
	}
	if !r.tt.HasText("Not set") {
		t.Error("a shortcut with no keys does not say Not set")
	}
	if !r.tt.Focused("Filter shortcuts") {
		t.Error("the filter did not take the focus")
	}
	r.tt.Key(0, ui.KeyDown)
	if !r.tt.Focused("Shortcuts") {
		t.Error("Down from the filter did not move into the list")
	}
	r.tt.Key(0, ui.KeyUp) // the list is at its top: Up goes back
	if !r.tt.Focused("Filter shortcuts") {
		t.Error("Up at the top of the list did not return to the filter")
	}
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	if r.m.help.open || !r.tt.Focused("Opener") {
		t.Errorf("Escape: open %v, focus back %v", r.m.help.open, r.tt.Focused("Opener"))
	}
}

func TestHelpFilterNarrowsShortcutsAsYouType(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Shift, ui.KeySlash)
	r.tt.Type("next")
	if !r.tt.HasText("Next track") || !r.tt.HasText("Playback") {
		t.Errorf("next: the match or its group is missing; texts %q", r.tt.Texts())
	}
	for _, s := range []string{"Play or pause", "Show or hide the sidebar", "View", "General"} {
		if r.tt.HasText(s) {
			t.Errorf("next: %q is still listed", s)
		}
	}
	if got := r.tt.Announcements(); !slices.Contains(got, "1 shortcut") {
		t.Errorf("announced %q, want the count", got)
	}
	// Keys match too, as spelt on the caps and in words, and so do keywords
	// and the group's title.
	for _, c := range []struct{ query, want string }{{"ctrl", "Next track"}, {"space", "Play or pause"}, {"resume", "Play or pause"}, {"pages", "Library · Albums"}, {"ARROW right", "Next track"}} {
		r.tt.Key(ui.Ctrl, ui.KeyA)
		r.tt.Type(c.query)
		if !r.tt.HasText(c.want) {
			t.Errorf("%q does not find %q; texts %q", c.query, c.want, r.tt.Texts())
		}
	}
	r.tt.Key(ui.Ctrl, ui.KeyA)
	r.tt.Type("zzz")
	if !r.tt.HasText("No shortcuts match “zzz”") || r.tt.HasText("Next track") {
		t.Errorf("zzz: %q", r.tt.Texts())
	}
	// Escape clears the filter first and leaves the sheet open; the next closes it.
	r.tt.Key(0, ui.KeyEscape)
	if !r.m.help.open || !r.tt.HasText("Next track") || !r.tt.HasText("Play or pause") {
		t.Errorf("Escape in a filled filter: open %v, texts %q", r.m.help.open, r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	if r.m.help.open {
		t.Error("Escape in an empty filter left the sheet open")
	}
}

func TestHelpQuestionMarkAfterTextIsText(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Shift, ui.KeySlash)
	r.tt.Type("n")
	r.tt.Key(ui.Shift, ui.KeySlash) // a "?" after text is text
	if !r.m.help.open {
		t.Error("a ? typed after other text closed the sheet")
	}
}

func TestHelpReflectsRebindingAtOnce(t *testing.T) {
	notSet := func(r *rig) int {
		n := 0
		for _, s := range r.tt.Texts() {
			if s == "Not set" {
				n++
			}
		}
		return n
	}
	r := newRig(t)
	r.tt.Key(ui.Shift, ui.KeySlash)
	before := notSet(r) // the sidebar's shortcut has no keys
	r.extra[1].Keys = nil
	r.tt.Frame()
	if after := notSet(r); after != before+1 {
		t.Errorf("Not set shown %d times, then %d after unbinding Next track", before, after)
	}
}

func TestHelpsOwnChordClosesItAndSettingsLinkOpensSettings(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Shift, ui.KeySlash)
	r.tt.Type("?") // the filter has the focus, where "?" is text: alone in it, it is the chord
	r.settle()
	if r.m.help.open {
		t.Fatal("? typed into the empty filter did not close the open help")
	}
	r.tt.Key(ui.Shift, ui.KeySlash)
	r.tt.Key(0, ui.KeyDown) // the focus is in the list: the chord itself
	r.tt.Key(ui.Shift, ui.KeySlash)
	r.settle()
	if r.m.help.open {
		t.Fatal("? did not close the open help from its list")
	}
	r.tt.Key(ui.Shift, ui.KeySlash)
	r.tt.MustClick("Settings")
	r.settle()
	if r.m.help.open {
		t.Error("the Settings link left the help open")
	}
	if got := r.router.Location(); got != "/settings?section=shortcuts" {
		t.Errorf("the link went to %q", got)
	}
}

func TestHelpShowsSettingsAsPlainTextWithoutThatPage(t *testing.T) {
	r := newRig(t)
	r.routes = []string{"/library/albums"}
	r.tt.Key(ui.Shift, ui.KeySlash)
	if !r.tt.HasText("Settings") {
		t.Fatal("the line about Settings is gone")
	}
	r.tt.MustClick("Settings")
	if got := r.router.Location(); got != "/library/albums" {
		t.Errorf("plain text navigated to %q", got)
	}
}

func TestOnlyOneSurfaceIsOpenAtATime(t *testing.T) {
	r := newRig(t)
	r.tt.Key(ui.Ctrl, ui.KeyK)
	r.m.toggleHelp()
	r.tt.Frame()
	if r.m.pal.IsOpen() || !r.m.help.open {
		t.Errorf("palette %v, help %v after opening the help over the palette", r.m.pal.IsOpen(), r.m.help.open)
	}
	r.m.togglePalette()
	r.tt.Frame()
	if !r.m.pal.IsOpen() || r.m.help.open {
		t.Errorf("palette %v, help %v after opening the palette over the help", r.m.pal.IsOpen(), r.m.help.open)
	}
}

func TestModuleDeclaresItsTwoShortcuts(t *testing.T) {
	m := New(uitest.Env(t, uitest.Bare()))
	got := map[string]module.Shortcut{}
	for _, s := range m.Shortcuts {
		got[s.ID] = s
	}
	p, h := got["commands.palette"], got["commands.help"]
	if p.Label() != "Open the command palette" || p.Group != "general" || !slices.Equal(p.Keys, []string{"Ctrl+K"}) || p.Scope != "global" || !p.NoPalette {
		t.Errorf("commands.palette: %+v", p)
	}
	if h.Label() != "Keyboard shortcuts" || h.Group != "general" || !slices.Equal(h.Keys, []string{"?"}) || h.Keywords() != "help keys bindings" || h.NoPalette {
		t.Errorf("commands.help: %+v", h)
	}
	if len(m.Routes) != 0 || m.Slots.Layer == nil {
		t.Errorf("routes %v, layer %v", m.Routes, m.Slots.Layer != nil)
	}
}

func TestGoldenPaletteAndHelp(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		opts := []uitest.Option{}
		if dark {
			name, opts = "dark", append(opts, uitest.Dark())
		}
		r := newRig(t, opts...)
		r.tt.Key(ui.Ctrl, ui.KeyK)
		r.settle()
		uitest.Golden(t, r.tt.Image(), "palette-"+name)
		r.m.toggleHelp()
		r.tt.Frame()
		r.settle()
		uitest.Golden(t, r.tt.Image(), "help-"+name)
	}
}
