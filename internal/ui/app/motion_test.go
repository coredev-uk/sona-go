package app

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func routeModule(patterns ...string) module.Factory {
	return func(*module.Env) module.Module {
		m := module.Module{ID: "routes"}
		for _, p := range patterns {
			m.Routes = append(m.Routes, module.Route{Pattern: p, Build: func(c *ui.Context, _ *ui.Route) { ui.Text(c, "page") }})
		}
		return m
	}
}

func TestEnvHasRouteMatchesRegisteredPatterns(t *testing.T) {
	a, _ := newShell(t, Options{}, routeModule("/library/albums", "/album/{id}", "/settings/{section...}", "/"))
	for path, want := range map[string]bool{
		"/library/albums":          true,
		"/library/albums/":         true,
		"/library/albums?sort=a":   true,
		"/library/albums#x":        true,
		"/library":                 false,
		"/library/albums/extra":    false,
		"/library/songs":           false,
		"/album/42":                true,
		"/album/42?x=1":            true,
		"/album":                   false,
		"/album/42/tracks":         false,
		"/settings":                true,
		"/settings/appearance":     true,
		"/settings/a/b/c":          true,
		"/artist/1":                false,
		"/":                        true,
		"":                         true,
		"/Library/Albums":          false,
		"/album/":                  false,
		"/settings/appearance?q=1": true,
	} {
		if got := a.env.HasRoute(path); got != want {
			t.Errorf("HasRoute(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestEnvHasRouteIsFalseWithoutModules(t *testing.T) {
	a, _ := newShell(t, Options{})
	if a.env.HasRoute("/library/albums") {
		t.Error("HasRoute is true with no module registered")
	}
}

func TestRouteMatchesWildcardNeedsAPart(t *testing.T) {
	for _, tc := range []struct {
		pattern, path string
		want          bool
	}{
		{"/a/{id}", "/a", false},
		{"/a/{id}", "/a/", false},
		{"/a/{id}/b", "/a/1/b", true},
		{"/a/{id}/b", "/a/1/c", false},
		{"/a/{rest...}", "/a", true},
		{"/a/{rest...}", "/b", false},
	} {
		if got := routeMatches(tc.pattern, tc.path); got != tc.want {
			t.Errorf("routeMatches(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// coverModule has a layer, an overlay and a page with a dialog, each opened
// by a flag the test sets.
type coverFlags struct{ layer, overlay, dialog bool }

func coverModule(f *coverFlags) module.Factory {
	return func(*module.Env) module.Module {
		return module.Module{
			ID: "cover",
			Slots: module.Slots{
				Layer:     func(c *ui.Context) { ui.Text(c, "layer") },
				LayerOpen: func() bool { return f.layer },
				Overlay:   &module.Overlay{Open: func() bool { return f.overlay }, Build: func(c *ui.Context) { ui.Text(c, "overlay") }},
			},
			Routes: []module.Route{{Pattern: "/p", Build: func(c *ui.Context, _ *ui.Route) {
				ui.Text(c, "page")
				kit.Dialog(c, &f.dialog, kit.DialogOpts{Title: "Rename"}, func() { ui.Text(c, "dialog body") })
			}}},
			Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "P" }, To: "/p"}},
		}
	}
}

func TestMotionPausesWhileALayerOverlayOrDialogIsOpen(t *testing.T) {
	t.Cleanup(kit.CancelConfirm)
	f := &coverFlags{}
	a, tt := newShell(t, Options{}, coverModule(f))
	m := a.env.Motion
	if m == nil {
		t.Fatal("the Env has no motion policy")
	}
	tt.Frame()
	if m.Covered() {
		t.Fatal("covered with nothing open")
	}
	for name, flag := range map[string]*bool{"a layer": &f.layer, "an overlay": &f.overlay, "a dialog": &f.dialog} {
		*flag = true
		tt.Frame()
		if !m.Covered() {
			t.Errorf("not covered while %s is open", name)
		}
		tt.Frame()
		if !m.Covered() {
			t.Errorf("a second frame uncovered %s", name)
		}
		*flag = false
		tt.Frame()
		if m.Covered() {
			t.Errorf("still covered after %s closed", name)
		}
	}

	kit.Confirm(kit.ConfirmOptions{Title: "Delete it?", ConfirmLabel: "Delete"}, func(bool) {})
	tt.Frame()
	if !m.Covered() {
		t.Error("not covered while a confirm question is open")
	}
	kit.CancelConfirm()
	tt.Frame()
	if m.Covered() {
		t.Error("still covered after the question was answered")
	}

	// Two at once nest: one cover, released with the last.
	f.layer, f.dialog = true, true
	tt.Frame()
	f.layer = false
	tt.Frame()
	if !m.Covered() {
		t.Error("the dialog stopped covering when the layer closed")
	}
	f.dialog = false
	tt.Frame()
	if m.Covered() {
		t.Error("covered with everything closed")
	}
}

func TestMotionReportReachesTheServiceOnceSettledAndOnChange(t *testing.T) {
	h := uitest.New(t, uitest.Bare())
	a := newApp(Options{Now: h.Clock.Now}, []module.Factory{pageModule("a", pageDef{"/p", "P", module.NavPrimary, 1})})
	t.Cleanup(a.env.Art.Close)
	t.Cleanup(a.env.Motion.Close)
	a.connect()
	var got []services.MotionReport
	a.sendReport = func(r services.MotionReport) { got = append(got, r) }
	tt := h.Tester(t, a.view, 1132, 800)

	tt.Frame()
	if len(got) != 0 {
		t.Fatalf("reported %d times inside the settle time", len(got))
	}
	h.Clock.Advance(reportSettle + time.Second)
	tt.Frame()
	if len(got) != 1 {
		t.Fatalf("reported %d times after settling, want 1", len(got))
	}
	if !got[0].Visible || !got[0].Setting {
		t.Errorf("report %+v: the window shows and the setting is on", got[0])
	}
	tt.Frame()
	if len(got) != 1 {
		t.Errorf("an unchanged report was sent again (%d)", len(got))
	}
	a.env.Motion.SetVisible(false)
	tt.Frame()
	if len(got) != 2 || got[1].Visible {
		t.Errorf("a hidden window was not reported: %+v", got)
	}
}

func TestMotionReportLandsInTheDiagnosticsBundle(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now},
		[]module.Factory{pageModule("a", pageDef{"/p", "P", module.NavPrimary, 1})})
	t.Cleanup(a.env.Art.Close)
	t.Cleanup(a.env.Motion.Close)
	a.firstPaint = nil
	a.connect()
	tt := h.Tester(t, a.view, 1132, 800)
	tt.Frame()
	h.Clock.Advance(reportSettle + time.Second)
	a.env.Motion.SetVisible(false)
	tt.Frame()
	b, err := services.Service[services.SystemService](h.Core).DiagnosticsBundle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`"webView":\s*\{`).MatchString(b.Text) || !regexp.MustCompile(`"visible":\s*false`).MatchString(b.Text) {
		t.Errorf("the bundle has no motion report from the shell:\n%s", b.Text)
	}
}
