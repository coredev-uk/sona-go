package app

import (
	"context"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	appleurl "github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// The menu bar is macOS's: the menu bar belongs to the app there, and an app
// that sets none has the default one, with nothing of its own in it. Linux
// and Windows keep no menu bar (Sona draws its own top bar).
//
// The model is plain data (menuEntry), built by a pure function
// (buildMenuBar) from what the shell knows: the shortcuts with the keys they
// answer to now (Env.Shortcuts: the user's bindings over the defaults), the
// sidebar's pages and what plays. An item runs the shortcut's own Run, the
// one the key and the command palette run, and is disabled while its When
// does not hold or it is Unavailable. menuBar writes the model into MyGo's
// menus and keeps them current.

// menuEntry is one item of the menu bar as data.
type menuEntry struct {
	// ID is unique in the bar: the ID of the shortcut the item runs, or the
	// shell's own ("role:cut", "menu:file", "shell:open-link").
	ID    string
	Label string
	// Role is the behaviour MyGo gives the item (Cut, Quit, Hide…); the
	// label is still ours, in the language shown.
	Role mygo.MenuRole
	// Type is checkbox for an item with a check mark, separator for a
	// separator; normal otherwise.
	Type mygo.MenuItemType
	// Accel is the item's key equivalent in MyGo's spelling ("Cmd+K"), ""
	// for none (see macAccelerator).
	Accel    string
	Disabled bool
	Checked  bool
	// Sub is the menu of a submenu; non-nil, even when empty, for one.
	Sub []menuEntry
	run func()
}

// menuPlayback is what plays, as far as the menus show it.
type menuPlayback struct {
	playing, shuffle, muted bool
	repeat                  string // off, all or one
}

// menuActions are the commands of the menu bar that no module registers as
// a shortcut; nil leaves an item out (no updates service, no settings page).
type menuActions struct {
	checkUpdates, openLink, reportBug, showMain func()
}

// menuInput is everything the menu bar is built from.
type menuInput struct {
	// commands are the shortcuts the menu items run, with their keys now
	// (Env.Shortcuts); one that is not registered leaves its item out.
	commands []module.BoundShortcut
	nav      [3][]module.NavItem
	play     menuPlayback
	// sidebar, miniShown and fullScreen are whether those show.
	sidebar, miniShown, fullScreen bool
	actions                        menuActions
}

// menuBuilder holds the lookups of one build.
type menuBuilder struct {
	in  menuInput
	cmd map[string]module.BoundShortcut
}

// buildMenuBar is the macOS menu bar: Sona, File, Edit, View, Controls,
// Window and Help. It reads the commands' When and Unavailable once, for the
// enabled states.
func buildMenuBar(in menuInput) []menuEntry {
	b := &menuBuilder{in: in, cmd: make(map[string]module.BoundShortcut, len(in.commands))}
	for _, s := range in.commands {
		b.cmd[s.ID] = s
	}
	return []menuEntry{b.appMenu(), b.fileMenu(), b.editMenu(), b.viewMenu(), b.controlsMenu(), b.windowMenu(), b.helpMenu()}
}

func (b *menuBuilder) appMenu() menuEntry {
	return menu("menu:app", mygo.RoleAppMenu, i18n.T("Sona"), group(
		slices.Concat(
			[]menuEntry{role(mygo.RoleAbout, i18n.T("About Sona"))},
			b.own("shell:check-updates", i18n.T("Check for Updates…"), b.in.actions.checkUpdates),
		),
		b.command("nav.settings", i18n.T("Settings…")),
		// Services is a submenu the system fills.
		[]menuEntry{{ID: "role:services", Role: mygo.RoleServices, Label: i18n.T("Services"), Sub: []menuEntry{}}},
		[]menuEntry{
			role(mygo.RoleHide, i18n.T("Hide Sona")),
			role(mygo.RoleHideOthers, i18n.T("Hide Others")),
			role(mygo.RoleUnhide, i18n.T("Show All")),
		},
		[]menuEntry{role(mygo.RoleQuit, i18n.T("Quit Sona"))},
	))
}

func (b *menuBuilder) fileMenu() menuEntry {
	return menu("menu:file", "", i18n.T("File"), group(
		slices.Concat(
			b.command("playlist.new", i18n.T("New Playlist")),
			b.own("shell:open-link", i18n.T("Open Link from Clipboard…"), b.in.actions.openLink),
		),
		[]menuEntry{role(mygo.RoleClose, i18n.T("Close Window"))},
	))
}

// editMenu holds the editing roles: MyGo enables each while the focus can
// act on it, and a text field takes them from here on macOS.
func (b *menuBuilder) editMenu() menuEntry {
	return menu("menu:edit", "", i18n.T("Edit"), group(
		[]menuEntry{role(mygo.RoleUndo, i18n.T("Undo")), role(mygo.RoleRedo, i18n.T("Redo"))},
		[]menuEntry{
			role(mygo.RoleCut, i18n.T("Cut")),
			role(mygo.RoleCopy, i18n.T("Copy")),
			role(mygo.RolePaste, i18n.T("Paste")),
			role(mygo.RoleDelete, i18n.T("Delete")),
			role(mygo.RoleSelectAll, i18n.T("Select All")),
		},
	))
}

func (b *menuBuilder) viewMenu() menuEntry {
	sidebar := i18n.T("Show Sidebar")
	if b.in.sidebar {
		sidebar = i18n.T("Hide Sidebar")
	}
	var pages, library []menuEntry
	for _, n := range b.in.nav[module.NavPrimary] {
		pages = append(pages, b.command("nav."+navID(n.To), n.Label())...)
	}
	for _, n := range b.in.nav[module.NavLibrary] {
		library = append(library, b.command("nav."+navID(n.To), n.Label())...)
	}
	if len(library) > 0 {
		pages = append(pages, submenu("menu:library", i18n.T("Library"), library))
	}
	full := i18n.T("Enter Full Screen")
	if b.in.fullScreen {
		full = i18n.T("Exit Full Screen")
	}
	return menu("menu:view", "", i18n.T("View"), group(
		b.command("shell.sidebar", sidebar),
		pages,
		slices.Concat(
			b.command("player.now-playing", i18n.T("Now Playing")),
			b.command("player.lyrics", i18n.T("Lyrics")),
			b.command("player.queue", i18n.T("Queue")),
		),
		[]menuEntry{role(mygo.RoleToggleFullScreen, full)},
	))
}

func (b *menuBuilder) controlsMenu() menuEntry {
	p := b.in.play
	play := i18n.T("Play")
	if p.playing {
		play = i18n.T("Pause")
	}
	repeat := i18n.T("Repeat")
	switch p.repeat {
	case "all":
		repeat = i18n.T("Repeat All")
	case "one":
		repeat = i18n.T("Repeat One")
	}
	check := func(on bool) func(*menuEntry) {
		return func(e *menuEntry) { e.Type, e.Checked = mygo.MenuItemCheckbox, on }
	}
	sleep := slices.Concat(
		b.command("player.sleep-30", i18n.T("In 30 Minutes")),
		b.command("player.sleep-60", i18n.T("In 1 Hour")),
		b.command("player.sleep-track", i18n.T("After This Song")),
		b.command("player.sleep-queue", i18n.T("After the Queue")),
		b.command("player.sleep-off", i18n.T("Turn Off")),
	)
	var sleepMenu []menuEntry
	if len(sleep) > 0 {
		sub := submenu("menu:sleep", i18n.T("Sleep Timer"), sleep)
		sub.Disabled = !slices.ContainsFunc(sleep, func(e menuEntry) bool { return e.Type != mygo.MenuItemSeparator && !e.Disabled })
		sleepMenu = []menuEntry{sub}
	}
	return menu("menu:controls", "", i18n.T("Controls"), group(
		slices.Concat(
			b.command("player.toggle", play),
			b.command("player.next", i18n.T("Next")),
			b.command("player.previous", i18n.T("Previous")),
		),
		slices.Concat(
			b.command("player.shuffle", i18n.T("Shuffle"), check(p.shuffle)),
			b.command("player.repeat", repeat, check(p.repeat == "all" || p.repeat == "one")),
		),
		slices.Concat(
			b.command("player.volume-up", i18n.T("Volume Up")),
			b.command("player.volume-down", i18n.T("Volume Down")),
			b.command("player.mute", i18n.T("Mute"), check(p.muted)),
		),
		sleepMenu,
	))
}

func (b *menuBuilder) windowMenu() menuEntry {
	mini := i18n.T("Mini Player")
	if b.in.miniShown {
		mini = i18n.T("Close Mini Player")
	}
	return menu("menu:window", mygo.RoleWindowMenu, i18n.T("Window"), group(
		[]menuEntry{role(mygo.RoleMinimize, i18n.T("Minimize")), role(mygo.RoleZoom, i18n.T("Zoom"))},
		slices.Concat(
			b.command("miniplayer.toggle", mini),
			b.own("shell:show-main", i18n.T("Sona"), b.in.actions.showMain),
		),
		[]menuEntry{role(mygo.RoleFront, i18n.T("Bring All to Front"))},
	))
}

func (b *menuBuilder) helpMenu() menuEntry {
	return menu("menu:help", mygo.RoleHelp, i18n.T("Help"), group(slices.Concat(
		b.command("commands.help", i18n.T("Keyboard Shortcuts")),
		b.command("commands.palette", i18n.T("Command Palette")),
		b.own("shell:report-bug", i18n.T("Report a Bug…"), b.in.actions.reportBug),
	)))
}

// command is the item that runs the shortcut with this ID, or nothing when
// no module registered it. Its key equivalent is the shortcut's key now, the
// user's or the default, when a menu may own it (macAccelerator); it is
// disabled while the shortcut's When does not hold or it is Unavailable, as
// the key does nothing then. opts adjust the entry (a check mark).
func (b *menuBuilder) command(id, label string, opts ...func(*menuEntry)) []menuEntry {
	s, ok := b.cmd[id]
	if !ok {
		return nil
	}
	e := menuEntry{
		ID:       id,
		Label:    label,
		Accel:    menuAccelerator(s.Keys),
		Disabled: !allowed(s.Shortcut) || s.Unavailable != nil && s.Unavailable() != "",
		run: func() {
			if allowed(s.Shortcut) {
				invoke(s.Shortcut)
			}
		},
	}
	for _, o := range opts {
		o(&e)
	}
	return []menuEntry{e}
}

// own is an item of the shell's own that runs run; nothing for nil.
func (b *menuBuilder) own(id, label string, run func()) []menuEntry {
	if run == nil {
		return nil
	}
	return []menuEntry{{ID: id, Label: label, run: run}}
}

// role is an item with MyGo's behaviour for the role, labelled in the
// language shown.
func role(r mygo.MenuRole, label string) menuEntry {
	return menuEntry{ID: "role:" + string(r), Role: r, Label: label}
}

// menu is a menu of the bar.
func menu(id string, r mygo.MenuRole, label string, items []menuEntry) menuEntry {
	return menuEntry{ID: id, Role: r, Label: label, Sub: items}
}

// submenu is a menu inside another.
func submenu(id, label string, items []menuEntry) menuEntry { return menu(id, "", label, items) }

// group joins the groups of a menu with separators, leaving out empty ones.
func group(groups ...[]menuEntry) []menuEntry {
	out := []menuEntry{}
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, menuEntry{Type: mygo.MenuItemSeparator})
		}
		out = append(out, g...)
	}
	return out
}

// menuAccelerator is the first of the keys that a menu may own, as MyGo
// spells it ("Cmd+Shift+E"), or "" when there is none.
func menuAccelerator(keys []string) string {
	for _, k := range keys {
		if acc, ok := macAccelerator(k); ok {
			return acc
		}
	}
	return ""
}

// macAccelerator writes a bound chord as a key equivalent. A key equivalent
// is taken by the menu before the window sees the key, and the window's
// text fields are MyGo's own, not AppKit's, so the menu cannot step aside for
// them as it does for a Cocoa text field. A chord is therefore a key
// equivalent only when it is a Command chord that no text field edits with:
// not a lone key or Shift or Option with one (the letters and digits they
// type, and Option+1…9, which the sidebar's library links answer to), and
// not Command with an arrow, Home, End, Page Up or Down, Backspace or
// Delete, which move the caret and delete a line. Those keys keep working
// from the keyboard (the dispatcher), their items show no key.
func macAccelerator(chord string) (string, bool) {
	mods, key, ok := kit.ParseChord(chord)
	if !ok || mods&ui.Super == 0 {
		return "", false
	}
	switch key {
	case ui.KeySpace, ui.KeyTab, ui.KeyEscape, ui.KeyBackspace, ui.KeyDelete,
		ui.KeyUp, ui.KeyDown, ui.KeyLeft, ui.KeyRight,
		ui.KeyHome, ui.KeyEnd, ui.KeyPageUp, ui.KeyPageDown:
		return "", false
	}
	name, ok := kit.ChordKeyName(key)
	if !ok {
		return "", false
	}
	parts := []string{"Cmd"}
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, name), "+"), true
}

// menuBar is the model written into MyGo's menus. Main thread only.
type menuBar struct {
	a *App
	// menu is the bar in use; items its items by ID, runs what each runs and
	// shown the entries as last written, so a refresh only touches what
	// changed. shape is the structure: a refresh that changes it (an item
	// came or went) installs the bar again.
	menu  *mygo.Menu
	items map[string]*mygo.MenuItem
	runs  map[string]func()
	shown map[string]menuEntry
	shape []string
	// sidebar and fullScreen are what the labels were built for; dirty asks
	// for a refresh at the next frame (an item was chosen: a check mark the
	// menu flipped itself is written back).
	sidebar, fullScreen, dirty bool
}

// newMenuBar sets the bar of the app, on macOS; nil elsewhere.
func (a *App) newMenuBar() *menuBar {
	if runtime.GOOS != "darwin" {
		return nil
	}
	return &menuBar{a: a}
}

// menuInput is what the shell knows now.
func (a *App) menuInput() menuInput {
	st := a.env.Play.State()
	a.mu.Lock()
	mini := a.miniShown
	a.mu.Unlock()
	in := menuInput{
		nav:        a.nav,
		sidebar:    a.sidebar,
		miniShown:  mini,
		fullScreen: a.env.Window.FullScreen(),
		play: menuPlayback{
			playing: st.Current != nil && (st.State == "playing" || st.State == "loading"),
			shuffle: st.Shuffle,
			muted:   st.Volume == 0,
			repeat:  st.Repeat,
		},
		actions: a.menuActions(),
	}
	if a.keys != nil {
		in.commands = a.keys.bindings()
	}
	return in
}

// menuActions are the shell's own commands, where it can run them.
func (a *App) menuActions() menuActions {
	var acts menuActions
	if a.opts.Core != nil {
		acts.checkUpdates = a.checkForUpdates
	}
	acts.openLink = a.openClipboardLink
	if a.hasRoute("/settings") {
		acts.reportBug = func() { a.router.Push("/settings?section=diagnostics") }
	}
	if a.opts.Windows != nil {
		acts.showMain = a.opts.Windows.ShowMain
	}
	return acts
}

// refreshMenu writes the model of now into the menu bar.
func (a *App) refreshMenu() {
	if a.menubar != nil {
		a.menubar.refresh()
	}
}

// frame refreshes the bar when something the frame knows changed: the
// sidebar (a key toggles it), full screen, or an item was chosen.
func (m *menuBar) frame() {
	if m.dirty || m.sidebar != m.a.sidebar || m.fullScreen != m.a.env.Window.FullScreen() {
		m.refresh()
	}
}

// refresh builds the model and writes it: item by item while the structure
// is the same, else the whole bar.
func (m *menuBar) refresh() {
	in := m.a.menuInput()
	m.dirty, m.sidebar, m.fullScreen = false, in.sidebar, in.fullScreen
	entries := buildMenuBar(in)
	shape := shapeOf(entries, nil)
	if m.menu == nil || !slices.Equal(shape, m.shape) {
		m.install(entries, shape)
		return
	}
	m.apply(entries)
}

// install makes MyGo's menus of the model and sets them as the bar.
func (m *menuBar) install(entries []menuEntry, shape []string) {
	m.items = map[string]*mygo.MenuItem{}
	m.runs = map[string]func(){}
	m.shown = map[string]menuEntry{}
	m.shape = shape
	m.menu = mygo.NewMenu(m.toItems(entries))
	mygo.App.SetMenu(m.menu)
}

func (m *menuBar) toItems(entries []menuEntry) []*mygo.MenuItem {
	out := make([]*mygo.MenuItem, 0, len(entries))
	for _, e := range entries {
		if e.Type == mygo.MenuItemSeparator {
			out = append(out, mygo.Separator())
			continue
		}
		it := &mygo.MenuItem{
			ID: e.ID, Label: e.Label, Role: e.Role, Type: e.Type, Accelerator: e.Accel,
			Disabled: e.Disabled, Checked: e.Checked,
		}
		if e.Sub != nil {
			it.Submenu = m.toItems(e.Sub)
		}
		if e.run != nil {
			id := e.ID
			it.Click = func(*mygo.MenuItem, *mygo.Window) { m.click(id) }
		}
		m.items[e.ID] = it
		m.runs[e.ID] = e.run
		m.shown[e.ID] = e
		out = append(out, it)
	}
	return out
}

// click runs an item chosen in the menu bar as the key would: in the window's
// update, which redraws it, and then writes the bar again, so that a check
// mark the menu flipped itself shows what came of the command.
func (m *menuBar) click(id string) {
	run := m.runs[id]
	if run == nil {
		return
	}
	m.a.post(func() {
		run()
		m.dirty = true
	})
}

// apply writes what differs from the bar as it was last written.
func (m *menuBar) apply(entries []menuEntry) {
	for _, e := range entries {
		if e.Type == mygo.MenuItemSeparator {
			continue
		}
		if it, prev := m.items[e.ID], m.shown[e.ID]; it != nil {
			if e.Label != prev.Label {
				it.SetLabel(e.Label)
			}
			if e.Accel != prev.Accel {
				it.SetAccelerator(e.Accel)
			}
			if e.Disabled != prev.Disabled {
				it.SetEnabled(!e.Disabled)
			}
			if e.Type == mygo.MenuItemCheckbox && it.IsChecked() != e.Checked {
				it.SetChecked(e.Checked)
			}
		}
		m.runs[e.ID] = e.run
		m.shown[e.ID] = e
		m.apply(e.Sub)
	}
}

// shapeOf is the structure of entries: their IDs in order, a separator "-",
// a submenu's items between "(" and ")".
func shapeOf(entries []menuEntry, out []string) []string {
	for _, e := range entries {
		if e.Type == mygo.MenuItemSeparator {
			out = append(out, "-")
			continue
		}
		out = append(out, e.ID)
		if e.Sub != nil {
			out = append(out, "(")
			out = shapeOf(e.Sub, out)
			out = append(out, ")")
		}
	}
	return out
}

// checkForUpdates is Sona → Check for Updates…: the check runs, and its
// result shows where Settings → General shows it.
func (a *App) checkForUpdates() {
	if a.opts.Core == nil {
		return
	}
	updates := services.Service[services.UpdatesService](a.opts.Core)
	a.router.Push("/settings?section=general")
	go func() {
		st, err := updates.Check(context.Background())
		a.post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't check GitHub for updates"), i18n.ErrorMessage(err))
				a.env.Queries.Invalidate(store.KeyUpdates)
				return
			}
			store.Set(a.env.Queries, store.KeyUpdates, st)
		})
	}()
}

// openClipboardLink is File → Open Link from Clipboard…: the page of the
// Apple Music link that was copied, as a link Sona is launched with opens it.
func (a *App) openClipboardLink() {
	link := strings.TrimSpace(mygo.Clipboard.ReadText())
	if !appleurl.IsMusicURL(link) {
		kit.ToastErrorf(i18n.T("Couldn't open a link"), i18n.T("Copy an Apple Music link first."))
		return
	}
	t, err := appleurl.ParseURL(link)
	if err != nil || !a.openTarget(services.OpenTarget{Kind: string(t.Kind), ID: t.ID, AlbumID: t.AlbumID, Storefront: t.Storefront}) {
		kit.ToastErrorf(i18n.T("Couldn't open a link"), i18n.T("Sona has no page for that link."))
	}
}
