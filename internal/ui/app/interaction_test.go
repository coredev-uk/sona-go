package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The interaction-consistency review (#311) pins what the shell's sidebar
// links do today for each gesture. The rule being reviewed: choosing a result
// opens it; Play plays; a track list's row plays (double-click or Enter); a
// modifier never does something a visible cue does not hint at. Where today's
// behaviour departs from it, the assertion carries a "DEPARTS:" comment, to be
// found (and flipped) when the behaviour is fixed.

// A nav link (the primary pages, the library's, the footer's) is a link: a
// click opens its page, a double-click opens it once (the router keeps one
// history entry for the two pushes) and Enter on the focused link opens it.
// Nothing else does anything: a modifier on Enter, Space and a right-click
// leave the page alone and open no menu.
func TestInteractionSidebarNavLinks(t *testing.T) {
	a, tt := newShell(t, Options{}, pageModule("a",
		pageDef{"/home", "Home", module.NavPrimary, 1},
		pageDef{"/albums", "Albums", module.NavLibrary, 20},
		pageDef{"/songs", "Songs", module.NavLibrary, 30},
		pageDef{"/settings", "Settings", module.NavFooter, 1},
	), func(*module.Env) module.Module {
		// A page with no link of its own, where each gesture starts.
		return module.Module{ID: "start", Routes: []module.Route{{
			Pattern: "/start", Build: func(c *ui.Context, _ *ui.Route) { ui.Text(c, "page of Start") },
		}}}
	})
	tt.Frame()
	reset := func() {
		a.router.Push("/start")
		tt.Frame()
	}
	tab := func(label string) {
		t.Helper()
		for range 40 {
			if tt.Focused(label) {
				return
			}
			tt.Key(0, ui.KeyTab)
			tt.Frame()
		}
		t.Fatalf("Tab never reached %q", label)
	}
	for _, c := range []struct{ label, path string }{
		{"Home", "/home"}, {"Songs", "/songs"}, {"Settings", "/settings"},
	} {
		if info, ok := tt.Access(c.label); !ok || info.Role != ui.RoleLink {
			t.Errorf("%s: role %v (found %v), want a link", c.label, info.Role, ok)
		}
		r, ok := tt.Find(c.label)
		if !ok {
			t.Fatalf("no %q in %q", c.label, tt.Texts())
		}
		x, y := r.X+r.W/2, r.Y+r.H/2

		reset()
		tt.ClickAt(x, y)
		if got := a.router.Path(); got != c.path {
			t.Errorf("%s, click: at %q, want %q", c.label, got, c.path)
		}

		reset()
		tt.ClickAt(x, y)
		tt.ClickAt(x, y)
		if got := a.router.Path(); got != c.path {
			t.Errorf("%s, double-click: at %q, want %q", c.label, got, c.path)
		}
		a.router.Back()
		if got := a.router.Path(); got != "/start" {
			t.Errorf("%s, double-click: Back is at %q, want one history entry (/start)", c.label, got)
		}

		for _, k := range []struct {
			gesture string
			mods    ui.Modifiers
			key     ui.Key
			want    string
		}{
			{"Enter", 0, ui.KeyEnter, c.path},
			{"Shift+Enter", ui.Shift, ui.KeyEnter, "/start"},
			{"Space", 0, ui.KeySpace, "/start"},
		} {
			reset()
			tab(c.label)
			tt.Key(k.mods, k.key)
			if got := a.router.Path(); got != k.want {
				t.Errorf("%s, %s: at %q, want %q", c.label, k.gesture, got, k.want)
			}
		}

		reset()
		_ = tt.RightClick(c.label)
		if got := a.router.Path(); got != "/start" {
			t.Errorf("%s, right-click: at %q, want /start", c.label, got)
		}
		if n := len(access.Read(t, tt.Tester).Roles(ui.RoleMenuItem)); n != 0 {
			t.Errorf("%s, right-click opened a menu of %d items", c.label, n)
		}
	}
}
