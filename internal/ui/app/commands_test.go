package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/features/commands"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The commands feature over the real shell: its chords go through the
// dispatcher, and the palette lists what every module registered.

func shortcutModule(ran *int) module.Factory {
	return func(*module.Env) module.Module {
		return module.Module{ID: "x", Shortcuts: []module.Shortcut{{
			ID: "x.ping", Label: func() string { return "Ping the server" }, Group: "general", Keys: []string{"Ctrl+J"}, Scope: "global",
			Keywords: func() string { return "network" }, Icon: "play", Run: func() { *ran++ },
		}}}
	}
}

func TestCtrlKOpensThePaletteWithEveryModulesShortcutAndRunsOne(t *testing.T) {
	ran := 0
	_, tt := newShell(t, Options{}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}), shortcutModule(&ran), commands.New)
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyK)
	for _, s := range []string{"Ping the server", "Library · Albums", "Show or hide the sidebar", "Keyboard shortcuts"} {
		if !tt.HasText(s) {
			t.Errorf("the palette does not list %q; texts %q", s, tt.Texts())
		}
	}
	if tt.HasText("Open the command palette") {
		t.Error("the palette lists itself")
	}
	tt.Type("ping")
	tt.Key(0, ui.KeyEnter)
	tt.Advance(400 * time.Millisecond)
	if ran != 1 {
		t.Errorf("the command ran %d times", ran)
	}
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Key(ui.Cmd, ui.KeyK) // its own chord closes it
	tt.Advance(400 * time.Millisecond)
	if tt.HasText("Go to a page, run an action, find music in your library") {
		t.Error("Ctrl+K did not close the palette")
	}
}

func TestQuestionMarkOpensTheShortcutHelpWithEveryGroup(t *testing.T) {
	ran := 0
	_, tt := newShell(t, Options{}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}), shortcutModule(&ran), commands.New)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeySlash)
	for _, s := range []string{"Keyboard shortcuts", "Pages", "View", "General", "Ping the server", "Library · Albums", "Open the command palette"} {
		if !tt.HasText(s) {
			t.Errorf("the help does not list %q; texts %q", s, tt.Texts())
		}
	}
	tt.Key(ui.Shift, ui.KeySlash)
	tt.Advance(400 * time.Millisecond)
	if tt.HasText("Change any of them in") {
		t.Error("? did not close the help")
	}
}

func TestHelpAndPaletteFollowTheUsersBindings(t *testing.T) {
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *config.Settings) {
		s.Shortcuts = config.Shortcuts{"x.ping": "none", "shell.sidebar": "Ctrl+Shift+B"}
	}); err != nil {
		t.Fatal(err)
	}
	ran := 0
	_, tt := newShell(t, Options{Settings: st}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}), shortcutModule(&ran), commands.New)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeySlash)
	if !tt.HasText("Not set") {
		t.Errorf("a shortcut bound to none does not say Not set: %q", tt.Texts())
	}
	if !tt.HasText("Shift") {
		t.Errorf("the rebound sidebar shortcut shows no Shift key: %q", tt.Texts())
	}
}

// layerModule is a page with a button and a layer that builds nothing; open
// is its LayerOpen (nil: none).
func layerModule(clicked *int, open func() bool) module.Factory {
	return func(*module.Env) module.Module {
		return module.Module{
			ID: "layer",
			Routes: []module.Route{{Pattern: "/", Build: func(c *ui.Context, _ *ui.Route) {
				if kit.Button(c.Key("go"), "Go", kit.ButtonOpts{}).Clicked() {
					*clicked++
				}
			}}},
			Nav:   []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "Home" }, Icon: "music", To: "/", Order: 1}},
			Slots: module.Slots{Layer: func(*ui.Context) {}, LayerOpen: open},
		}
	}
}

// A layer that is closed, or has no LayerOpen, covers the window with an
// empty box that must not take its clicks.
func TestAClosedLayerDoesNotTakeTheWindowsClicks(t *testing.T) {
	for name, open := range map[string]func() bool{"no LayerOpen": nil, "closed": func() bool { return false }} {
		t.Run(name, func(t *testing.T) {
			clicked := 0
			_, tt := newShell(t, Options{}, layerModule(&clicked, open))
			tt.Frame()
			if err := tt.Click("Go"); err != nil {
				t.Fatal(err)
			}
			if clicked != 1 {
				t.Errorf("the page's button was clicked %d times", clicked)
			}
		})
	}
}

// slashModule is a page with a button and a text field, the shell's help
// (commands.New) beside it.
func slashModule(text *string) module.Factory {
	return func(*module.Env) module.Module {
		return module.Module{
			ID: "slash",
			Routes: []module.Route{{Pattern: "/", Build: func(c *ui.Context, _ *ui.Route) {
				ui.Column(c).Gap(8).Children(func() {
					kit.Button(c.Key("go"), "Go", kit.ButtonOpts{})
					ui.TextInput(c.Key("field"), text).Label("Field")
				})
			}}},
			Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "Home" }, Icon: "music", To: "/", Order: 1}},
		}
	}
}

// questionMark presses "?" as GTK and AppKit deliver it with MyGo v0.8.3:
// Shift and the slash key by its position, with no Key.
func questionMark(tt *uitest.Session) {
	tt.CodeDown(ui.Shift, ui.CodeSlash)
	tt.CodeUp(ui.Shift, ui.CodeSlash)
}

func helpOpen(tt *uitest.Session) bool { return tt.HasText("Change any of them in") }

// On GTK and AppKit "?" has no Key: the dispatcher reads it by its position,
// with nothing focused (the start, a click on the background), from a
// focused button, and the help's own "?" closes it again.
func TestQuestionMarkByPositionOpensAndClosesTheHelp(t *testing.T) {
	text := ""
	_, tt := newShell(t, Options{}, slashModule(&text), commands.New)
	tt.Frame()
	questionMark(tt)
	if !helpOpen(tt) {
		t.Fatal("? at the start did not open the help")
	}
	tt.Advance(400 * time.Millisecond) // the list takes the focus
	questionMark(tt)
	tt.Advance(400 * time.Millisecond)
	if helpOpen(tt) {
		t.Fatal("? did not close the help")
	}

	if err := tt.Click("Go"); err != nil {
		t.Fatal(err)
	}
	questionMark(tt)
	if !helpOpen(tt) {
		t.Fatal("? from a focused button did not open the help")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(400 * time.Millisecond)
	if helpOpen(tt) || !tt.Focused("Go") {
		t.Fatalf("Escape: help open %v, focus back on Go %v", helpOpen(tt), tt.Focused("Go"))
	}

	// A click on the background takes the focus from the button.
	tt.ClickAt(1000, 700)
	tt.Frame()
	questionMark(tt)
	if !helpOpen(tt) {
		t.Fatal("? after a click on the background did not open the help")
	}
}

// A "?" typed in a text field is text, not the help.
func TestQuestionMarkByPositionStaysOutOfTextFields(t *testing.T) {
	text := ""
	_, tt := newShell(t, Options{}, slashModule(&text), commands.New)
	tt.Frame()
	if err := tt.Click("Field"); err != nil {
		t.Fatal(err)
	}
	tt.CodeDown(ui.Shift, ui.CodeSlash)
	tt.Type("?")
	tt.CodeUp(ui.Shift, ui.CodeSlash)
	tt.Frame()
	if helpOpen(tt) || text != "?" {
		t.Fatalf("typing ? in a field: help open %v, text %q", helpOpen(tt), text)
	}
}

// The "?" by position is the chord "?", whatever has it: the help bound to
// another key leaves it, and the shortcut bound to it runs.
func TestQuestionMarkByPositionFollowsTheBindings(t *testing.T) {
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Update(func(s *config.Settings) {
		s.Shortcuts = config.Shortcuts{"commands.help": "F1", "x.ping": "?"}
	}); err != nil {
		t.Fatal(err)
	}
	ran := 0
	_, tt := newShell(t, Options{Settings: st}, shortcutModule(&ran), commands.New)
	tt.Frame()
	questionMark(tt)
	tt.Frame()
	if helpOpen(tt) || ran != 1 {
		t.Errorf("? bound to Ping: help open %v, Ping ran %d times", helpOpen(tt), ran)
	}
}
