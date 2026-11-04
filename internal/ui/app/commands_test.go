package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/features/commands"
	"github.com/coredev-uk/sona-go/internal/ui/module"
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
	tt.Key(ui.Ctrl, ui.KeyK)
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
	tt.Key(ui.Ctrl, ui.KeyK)
	tt.Key(ui.Ctrl, ui.KeyK) // its own chord closes it
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

func TestEnvShortcutsListsEffectiveKeysAndHasRouteMatchesPatterns(t *testing.T) {
	ran := 0
	a, _ := newShell(t, Options{}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}), shortcutModule(&ran), commands.New)
	got := map[string][]string{}
	for _, s := range a.env.Shortcuts() {
		got[s.ID] = s.Keys
	}
	if len(got["x.ping"]) != 1 || got["x.ping"][0] != "Ctrl+J" || got["commands.help"][0] != "?" || got["nav.albums"][0] != "Alt+1" {
		t.Errorf("shortcuts %v", got)
	}
	for path, want := range map[string]bool{"/albums": true, "/albums?sort=title": true, "/albums/x": false, "/songs": false} {
		if a.env.HasRoute(path) != want {
			t.Errorf("HasRoute(%q) = %v", path, !want)
		}
	}
	for pattern, path := range map[string]string{"/album/{id}": "/album/l.1", "/settings/{section...}": "/settings/shortcuts/more"} {
		if !routeMatches(pattern, path) {
			t.Errorf("%s does not match %s", pattern, path)
		}
	}
	if routeMatches("/album/{id}", "/album") || routeMatches("/album/{id}", "/album/1/2") {
		t.Error("a pattern matched a path of another length")
	}
}
