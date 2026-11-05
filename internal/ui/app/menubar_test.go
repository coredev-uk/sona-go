package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// describeMenu is the model as lines, "Menu > Item [Accelerator]" with
// "disabled" and "checked" where they apply, separators as "---".
func describeMenu(entries []menuEntry, path string) []string {
	var out []string
	for _, e := range entries {
		if e.Type == mygo.MenuItemSeparator {
			out = append(out, path+"---")
			continue
		}
		line := path + e.Label
		if e.Accel != "" {
			line += " [" + e.Accel + "]"
		}
		if e.Disabled {
			line += " disabled"
		}
		if e.Checked {
			line += " checked"
		}
		if e.Role != "" {
			line += " role=" + string(e.Role)
		}
		out = append(out, line)
		if e.Sub != nil {
			out = append(out, describeMenu(e.Sub, path+e.Label+" > ")...)
		}
	}
	return out
}

// find is the entry with this ID, wherever it is in the bar.
func findEntry(entries []menuEntry, id string) (menuEntry, bool) {
	for _, e := range entries {
		if e.ID == id {
			return e, true
		}
		if got, ok := findEntry(e.Sub, id); ok {
			return got, true
		}
	}
	return menuEntry{}, false
}

// bind is a shortcut that answers to keys (the macOS spelling).
func bind(id string, keys ...string) module.BoundShortcut {
	return module.BoundShortcut{Shortcut: module.Shortcut{ID: id, Keys: keys, Run: func() {}}, Keys: keys}
}

// menuNav is the sidebar's pages as the modules register them.
func menuNav() [3][]module.NavItem {
	item := func(to, label string) module.NavItem {
		return module.NavItem{To: to, Label: func() string { return label }}
	}
	var nav [3][]module.NavItem
	nav[module.NavPrimary] = []module.NavItem{item("/", "For you"), item("/new", "Discover"), item("/radio", "Radio"), item("/search", "Search")}
	nav[module.NavLibrary] = []module.NavItem{item("/library/songs", "Songs"), item("/library/albums", "Albums")}
	nav[module.NavFooter] = []module.NavItem{item("/settings", "Settings")}
	return nav
}

// menuCommands are the shortcuts of the menu bar with the keys they have on
// macOS by default.
func menuCommands() []module.BoundShortcut {
	return []module.BoundShortcut{
		bind("nav.settings", "Meta+,"),
		bind("nav.home", "Meta+1"), bind("nav.new", "Meta+2"), bind("nav.radio", "Meta+3"), bind("nav.search", "Meta+4"),
		bind("nav.library.songs", "Alt+1"), bind("nav.library.albums", "Alt+2"),
		bind("shell.sidebar", "Meta+B"),
		bind("playlist.new", "Meta+N"),
		bind("player.toggle", "Space"), bind("player.next", "Meta+ArrowRight"), bind("player.previous", "Meta+ArrowLeft"),
		bind("player.shuffle", "S"), bind("player.repeat", "R"),
		bind("player.volume-up", "Meta+ArrowUp"), bind("player.volume-down", "Meta+ArrowDown"), bind("player.mute", "M"),
		bind("player.sleep-30"), bind("player.sleep-60"), bind("player.sleep-track"), bind("player.sleep-queue"), bind("player.sleep-off"),
		bind("player.now-playing", "N"), bind("player.lyrics", "L"), bind("player.queue", "Q"),
		bind("miniplayer.toggle", "Meta+Shift+M"),
		bind("commands.help", "?"), bind("commands.palette", "Meta+K"),
	}
}

func allActions() menuActions {
	noop := func() {}
	return menuActions{checkUpdates: noop, openLink: noop, reportBug: noop, showMain: noop}
}

func TestMenuBarModel(t *testing.T) {
	got := describeMenu(buildMenuBar(menuInput{
		commands: menuCommands(), nav: menuNav(), sidebar: true, actions: allActions(),
		play: menuPlayback{repeat: "off"},
	}), "")
	want := []string{
		"Sona role=appMenu",
		"Sona > About Sona role=about",
		"Sona > Check for Updates…",
		"Sona > ---",
		"Sona > Settings… [Cmd+,]",
		"Sona > ---",
		"Sona > Services role=services",
		"Sona > ---",
		"Sona > Hide Sona role=hide",
		"Sona > Hide Others role=hideOthers",
		"Sona > Show All role=unhide",
		"Sona > ---",
		"Sona > Quit Sona role=quit",
		"File",
		"File > New Playlist [Cmd+N]",
		"File > Open Link from Clipboard…",
		"File > ---",
		"File > Close Window role=close",
		"Edit",
		"Edit > Undo role=undo",
		"Edit > Redo role=redo",
		"Edit > ---",
		"Edit > Cut role=cut",
		"Edit > Copy role=copy",
		"Edit > Paste role=paste",
		"Edit > Delete role=delete",
		"Edit > Select All role=selectAll",
		"View",
		"View > Hide Sidebar [Cmd+B]",
		"View > ---",
		"View > For you [Cmd+1]",
		"View > Discover [Cmd+2]",
		"View > Radio [Cmd+3]",
		"View > Search [Cmd+4]",
		"View > Library",
		"View > Library > Songs",
		"View > Library > Albums",
		"View > ---",
		"View > Now Playing",
		"View > Lyrics",
		"View > Queue",
		"View > ---",
		"View > Enter Full Screen role=togglefullscreen",
		"Controls",
		"Controls > Play",
		"Controls > Next",
		"Controls > Previous",
		"Controls > ---",
		"Controls > Shuffle",
		"Controls > Repeat",
		"Controls > ---",
		"Controls > Volume Up",
		"Controls > Volume Down",
		"Controls > Mute",
		"Controls > ---",
		"Controls > Sleep Timer",
		"Controls > Sleep Timer > In 30 Minutes",
		"Controls > Sleep Timer > In 1 Hour",
		"Controls > Sleep Timer > After This Song",
		"Controls > Sleep Timer > After the Queue",
		"Controls > Sleep Timer > Turn Off",
		"Window role=windowMenu",
		"Window > Minimize role=minimize",
		"Window > Zoom role=zoom",
		"Window > ---",
		"Window > Mini Player [Cmd+Shift+M]",
		"Window > Sona",
		"Window > ---",
		"Window > Bring All to Front role=front",
		"Help role=help",
		"Help > Keyboard Shortcuts",
		"Help > Command Palette [Cmd+K]",
		"Help > Report a Bug…",
	}
	if !slices.Equal(got, want) {
		t.Errorf("menu bar:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A lone key, Option or Shift with one, and Command with a key a text field
// edits with are no key equivalents: the menu would take them from the
// field's text.
func TestMenuAcceleratorsAreCommandChordsAMenuMayOwn(t *testing.T) {
	for _, c := range []struct{ chord, want string }{
		{"Meta+K", "Cmd+K"},
		{"Meta+Shift+E", "Cmd+Shift+E"},
		{"Meta+Alt+L", "Cmd+Alt+L"},
		{"Meta+,", "Cmd+,"},
		{"Meta+1", "Cmd+1"},
		{"Meta+Enter", "Cmd+Enter"},
		{"Meta+F5", "Cmd+F5"},
		{"Meta++", "Cmd+Shift+="},
		{"Space", ""},
		{"S", ""},
		{"?", ""},
		{"Shift+S", ""},
		{"Alt+1", ""},
		{"Alt+Shift+1", ""},
		{"Ctrl+K", ""}, // a saved Ctrl chord: Control types the caret's emacs keys
		{"Meta+Space", ""},
		{"Meta+ArrowLeft", ""},
		{"Meta+Shift+ArrowRight", ""},
		{"Meta+Backspace", ""},
		{"Meta+Home", ""},
		{"nonsense", ""},
	} {
		got, ok := macAccelerator(c.chord)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("macAccelerator(%q) = %q, %v; want %q", c.chord, got, ok, c.want)
		}
	}
	// The first key a menu may own, not the first key.
	if got := menuAccelerator([]string{"Space", "Meta+P", "Meta+Shift+P"}); got != "Cmd+P" {
		t.Errorf("menuAccelerator = %q", got)
	}
	if got := menuAccelerator(nil); got != "" {
		t.Errorf("menuAccelerator(nil) = %q", got)
	}
}

// The accelerators are the keys bound now: a rebinding in Settings → Shortcuts
// changes them, and "none" takes the key off the item.
func TestRebindingChangesTheMenuAccelerator(t *testing.T) {
	list := []module.Shortcut{
		{ID: "commands.palette", Keys: []string{"Ctrl+K"}, Run: func() {}},
		{ID: "player.toggle", Keys: []string{"Space"}, Run: func() {}},
		{ID: "shell.sidebar", Keys: []string{"Ctrl+B"}, Run: func() {}},
	}
	d := newDispatcher(platformKeys(list, ui.Super), nil)
	accel := func(id string) string {
		e, ok := findEntry(buildMenuBar(menuInput{commands: d.bindings()}), id)
		if !ok {
			t.Fatalf("no item for %s", id)
		}
		return e.Accel
	}
	if got := accel("commands.palette"); got != "Cmd+K" {
		t.Fatalf("default: %q", got)
	}
	d.rebind(config.Shortcuts{"commands.palette": "Meta+Shift+J", "player.toggle": "Meta+Alt+P", "shell.sidebar": "none"})
	for id, want := range map[string]string{"commands.palette": "Cmd+Shift+J", "player.toggle": "Cmd+Alt+P", "shell.sidebar": ""} {
		if got := accel(id); got != want {
			t.Errorf("%s after the rebinding: %q, want %q", id, got, want)
		}
	}
	// A binding that is no chord keeps the default; a lone key leaves the
	// item with none.
	d.rebind(config.Shortcuts{"commands.palette": "Hyper+K", "player.toggle": "P"})
	if got := accel("commands.palette"); got != "Cmd+K" {
		t.Errorf("invalid binding: %q", got)
	}
	if got := accel("player.toggle"); got != "" {
		t.Errorf("lone key: %q", got)
	}
}

// An item is enabled as its shortcut is: its When holds and it is not
// Unavailable. An item chosen does what the key does.
func TestMenuItemsFollowAndRunTheirShortcuts(t *testing.T) {
	var ran []string
	run := func(id string) func() { return func() { ran = append(ran, id) } }
	live, playing := false, true
	commands := []module.BoundShortcut{
		{Shortcut: module.Shortcut{ID: "player.toggle", Run: run("toggle")}},
		{Shortcut: module.Shortcut{ID: "player.next", Unavailable: func() string {
			if live {
				return "Not available while listening to a live station"
			}
			return ""
		}, Run: run("next")}},
		{Shortcut: module.Shortcut{ID: "player.mute", When: func() bool { return playing }, Run: run("mute")}},
		{Shortcut: module.Shortcut{ID: "player.sleep-30", When: func() bool { return playing }, Run: run("sleep-30")}},
		{Shortcut: module.Shortcut{ID: "player.sleep-off", When: func() bool { return false }, Run: run("sleep-off")}},
	}
	build := func() []menuEntry { return buildMenuBar(menuInput{commands: commands}) }
	state := func(id string) (menuEntry, bool) { return findEntry(build(), id) }

	for id, disabled := range map[string]bool{"player.toggle": false, "player.next": false, "player.mute": false, "menu:sleep": false, "player.sleep-off": true} {
		if e, ok := state(id); !ok || e.Disabled != disabled {
			t.Errorf("%s: disabled %v (found %v), want %v", id, e.Disabled, ok, disabled)
		}
	}
	live, playing = true, false
	for id, disabled := range map[string]bool{"player.toggle": false, "player.next": true, "player.mute": true, "player.sleep-30": true, "menu:sleep": true} {
		if e, ok := state(id); !ok || e.Disabled != disabled {
			t.Errorf("stopped, live: %s: disabled %v, want %v", id, e.Disabled, disabled)
		}
	}

	live, playing = false, true
	entries := build()
	for _, id := range []string{"player.toggle", "player.next", "player.mute", "player.sleep-30"} {
		e, _ := findEntry(entries, id)
		e.run()
	}
	if want := []string{"toggle", "next", "mute", "sleep-30"}; !slices.Equal(ran, want) {
		t.Errorf("ran %q, want %q", ran, want)
	}
	// A command whose When stopped holding after the menu was written does
	// nothing, as its key does nothing.
	ran, playing = nil, false
	e, _ := findEntry(entries, "player.mute")
	e.run()
	if len(ran) != 0 {
		t.Errorf("ran %q while its When did not hold", ran)
	}
}

// What plays shows in the labels and the check marks.
func TestControlsShowWhatPlays(t *testing.T) {
	describe := func(p menuPlayback) []string {
		var out []string
		for _, e := range buildMenuBar(menuInput{commands: menuCommands(), play: p}) {
			if e.ID == "menu:controls" {
				out = describeMenu(e.Sub, "")
			}
		}
		return out
	}
	at := func(lines []string, prefix string) string {
		for _, l := range lines {
			if strings.HasPrefix(l, prefix) {
				return l
			}
		}
		t.Fatalf("no %q in %q", prefix, lines)
		return ""
	}
	idle := describe(menuPlayback{repeat: "off"})
	if got := at(idle, "Play"); got != "Play" {
		t.Errorf("idle: %q", got)
	}
	for _, c := range []struct{ prefix, want string }{{"Shuffle", "Shuffle"}, {"Repeat", "Repeat"}, {"Mute", "Mute"}} {
		if got := at(idle, c.prefix); got != c.want {
			t.Errorf("idle: %q, want %q", got, c.want)
		}
	}
	busy := describe(menuPlayback{playing: true, shuffle: true, muted: true, repeat: "one"})
	for _, c := range []struct{ prefix, want string }{
		{"Pause", "Pause"}, {"Shuffle", "Shuffle checked"}, {"Repeat", "Repeat One checked"}, {"Mute", "Mute checked"},
	} {
		if got := at(busy, c.prefix); got != c.want {
			t.Errorf("playing: %q, want %q", got, c.want)
		}
	}
	if got := at(describe(menuPlayback{repeat: "all"}), "Repeat"); got != "Repeat All checked" {
		t.Errorf("repeat all: %q", got)
	}

	// The sidebar, the mini player and full screen word their toggles.
	view := func(in menuInput) []string {
		in.commands = menuCommands()
		var lines []string
		for _, l := range describeMenu(buildMenuBar(in), "") {
			if strings.Contains(l, "Sidebar") || strings.Contains(l, "Mini Player") || strings.Contains(l, "Full Screen") {
				lines = append(lines, l)
			}
		}
		return lines
	}
	if got, want := view(menuInput{}), []string{"View > Show Sidebar [Cmd+B]", "View > Enter Full Screen role=togglefullscreen", "Window > Mini Player [Cmd+Shift+M]"}; !slices.Equal(got, want) {
		t.Errorf("toggles off: %q, want %q", got, want)
	}
	if got, want := view(menuInput{sidebar: true, miniShown: true, fullScreen: true}), []string{
		"View > Hide Sidebar [Cmd+B]", "View > Exit Full Screen role=togglefullscreen", "Window > Close Mini Player [Cmd+Shift+M]",
	}; !slices.Equal(got, want) {
		t.Errorf("toggles on: %q, want %q", got, want)
	}
}

// What no module registers is not in the menus, and no menu is left with a
// separator first, last or doubled.
func TestMenuBarLeavesOutWhatIsNotThere(t *testing.T) {
	bar := buildMenuBar(menuInput{})
	var labels []string
	for _, m := range bar {
		labels = append(labels, m.Label)
	}
	if want := []string{"Sona", "File", "Edit", "View", "Controls", "Window", "Help"}; !slices.Equal(labels, want) {
		t.Errorf("menus %q, want %q", labels, want)
	}
	for _, line := range describeMenu(bar, "") {
		for _, gone := range []string{"Settings…", "New Playlist", "Play", "Pause", "Sleep Timer", "Mini Player", "Command Palette", "Report a Bug", "Check for Updates", "Open Link"} {
			if strings.HasSuffix(line, "> "+gone) || strings.Contains(line, "> "+gone+" ") {
				t.Errorf("%q is in the menus without a command behind it", line)
			}
		}
	}
	var check func(path string, items []menuEntry)
	check = func(path string, items []menuEntry) {
		for i, e := range items {
			if e.Type == mygo.MenuItemSeparator && (i == 0 || i == len(items)-1 || items[i-1].Type == mygo.MenuItemSeparator) {
				t.Errorf("%s: separator at %d of %d", path, i, len(items))
			}
			check(path+" > "+e.Label, e.Sub)
		}
	}
	check("", bar)
	check("", buildMenuBar(menuInput{commands: menuCommands(), nav: menuNav(), actions: allActions()}))
}

// The labels are said in the language shown.
func TestMenuBarFollowsTheLanguage(t *testing.T) {
	resetLanguage(t)
	in := menuInput{commands: menuCommands(), nav: menuNav(), actions: allActions()}
	english := describeMenu(buildMenuBar(in), "")
	i18n.SetLocale("en-XA")
	pseudo := describeMenu(buildMenuBar(in), "")
	if len(english) != len(pseudo) {
		t.Fatalf("%d lines in English, %d in the pseudo-locale", len(english), len(pseudo))
	}
	// Pages are the modules' own words (menuNav's labels are fixed text);
	// everything else was looked up.
	for i, e := range english {
		if strings.Contains(e, "---") || strings.Contains(e, "For you") || strings.Contains(e, "Discover") || strings.Contains(e, "Radio") ||
			strings.Contains(e, "Search") || strings.Contains(e, "Songs") || strings.Contains(e, "Albums") {
			continue
		}
		if e == pseudo[i] {
			t.Errorf("%q is not translated", e)
		}
	}
}

// The real modules: every command the menus run is registered, and the keys
// shown are the defaults as macOS binds them, then the user's.
func TestMenuBarOverTheRealModules(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now}, factories)
	t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
	a.firstPaint = nil
	a.connect()

	// The keys of macOS, whatever the platform the test runs on.
	bindings := func() []module.BoundShortcut {
		return newDispatcher(platformKeys(a.keys.shortcuts, ui.Super), a.keys.overrides).bindings()
	}
	in := a.menuInput()
	in.commands = bindings()
	bar := buildMenuBar(in)

	// Every item that runs a shortcut is there: none was left out for want
	// of a registered command.
	for _, id := range []string{
		"nav.settings", "playlist.new", "shell.sidebar", "nav.home", "nav.new", "nav.radio", "nav.search", "nav.library.songs", "nav.library.albums",
		"player.now-playing", "player.lyrics", "player.queue",
		"player.toggle", "player.next", "player.previous", "player.shuffle", "player.repeat",
		"player.volume-up", "player.volume-down", "player.mute", "menu:sleep",
		"player.sleep-30", "player.sleep-60", "player.sleep-track", "player.sleep-queue", "player.sleep-off",
		"miniplayer.toggle", "commands.help", "commands.palette", "shell:show-main", "shell:report-bug", "shell:check-updates", "shell:open-link",
	} {
		if _, ok := findEntry(bar, id); !ok && id != "shell:show-main" { // no Windows in tests
			t.Errorf("no item for %s", id)
		}
	}
	accel := func(id string) string { e, _ := findEntry(bar, id); return e.Accel }
	for id, want := range map[string]string{
		"nav.settings": "Cmd+,", "playlist.new": "Cmd+N", "shell.sidebar": "Cmd+B", "nav.home": "Cmd+1", "nav.search": "Cmd+5",
		"commands.palette": "Cmd+K", "miniplayer.toggle": "Cmd+Shift+M",
		// Keys a menu may not own show none.
		"player.toggle": "", "player.next": "", "player.volume-up": "", "player.mute": "", "nav.library.songs": "",
	} {
		if got := accel(id); got != want {
			t.Errorf("%s: %q, want %q", id, got, want)
		}
	}

	// A binding the user saves reaches the menu through settings:changed.
	next, err := h.Settings.Update(func(s *config.Settings) {
		s.Shortcuts = config.Shortcuts{"commands.palette": "Meta+Shift+K", "playlist.new": "none"}
	})
	if err != nil {
		t.Fatal(err)
	}
	h.Bus.Emit(services.EventSettingsChanged, next)
	h.Main.Drain()
	in.commands = bindings()
	bar = buildMenuBar(in)
	if got := accel("commands.palette"); got != "Cmd+Shift+K" {
		t.Errorf("rebound palette: %q", got)
	}
	if got := accel("playlist.new"); got != "" {
		t.Errorf("unbound New Playlist: %q", got)
	}

	// Choosing an item runs what the palette runs.
	e, _ := findEntry(bar, "playlist.new")
	if e.run == nil {
		t.Fatal("New Playlist runs nothing")
	}
}
