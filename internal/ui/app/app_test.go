package app

import (
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// pageModule is a module with one page per path, a nav item each, and a
// page body naming the page.
func pageModule(id string, pages ...pageDef) module.Factory {
	return func(*module.Env) module.Module {
		m := module.Module{ID: id}
		for _, p := range pages {
			m.Routes = append(m.Routes, module.Route{
				Pattern: p.path,
				Title:   func(*ui.Route) string { return p.title },
				Build: func(c *ui.Context, _ *ui.Route) {
					ui.Text(c, "page of "+p.title)
				},
			})
			m.Nav = append(m.Nav, module.NavItem{
				Section: p.section, Label: func() string { return p.title }, Icon: "music", To: p.path, Order: p.order,
			})
		}
		return m
	}
}

type pageDef struct {
	path, title string
	section     module.NavSection
	order       int
}

func newShell(t *testing.T, o Options, f ...module.Factory) (*App, *uitest.Session) {
	t.Helper()
	return newShellPaints(t, o, new(int), f...)
}

// newShellPaints is newShell counting first-paint reports in paints.
func newShellPaints(t *testing.T, o Options, paints *int, f ...module.Factory) (*App, *uitest.Session) {
	t.Helper()
	h := uitest.New(t, uitest.Bare()) // the light theme, restored afterwards
	o.Now = h.Clock.Now
	a := newApp(o, f)
	a.firstPaint = func() { *paints++ }
	t.Cleanup(a.env.Art.Close)
	a.connect()
	return a, h.Tester(t, a.view, 1132, 800)
}

func TestShellShowsTheRegisteredNavEntriesAndNothingElse(t *testing.T) {
	_, tt := newShell(t, Options{}, pageModule("a",
		pageDef{"/songs", "Songs", module.NavLibrary, 30},
		pageDef{"/albums", "Albums", module.NavLibrary, 20},
		pageDef{"/settings", "Settings", module.NavFooter, 1},
	))
	tt.Frame()
	want := []string{"Albums", "Songs", "Settings", "Library", "Sona"}
	for _, w := range want {
		if !tt.HasText(w) {
			t.Errorf("shell does not show %q; texts %q", w, tt.Texts())
		}
	}
	// What no module registered is not there: no Home, Search, Playlists…
	chrome := []string{"Main", "Back", "Forward", "page of Albums"} // the sidebar's name, the history buttons, the page
	for _, text := range tt.Texts() {
		if !slices.Contains(want, text) && !slices.Contains(chrome, text) {
			t.Errorf("shell shows %q, which no module registered", text)
		}
	}
	// Albums, the first library link, is the page it opens on.
	if !tt.HasText("page of Albums") {
		t.Errorf("start page is not Albums: %q", tt.Texts())
	}
}

func TestNavClickAndChordsNavigate(t *testing.T) {
	a, tt := newShell(t, Options{}, pageModule("a",
		pageDef{"/home", "Home", module.NavPrimary, 1},
		pageDef{"/search", "Search", module.NavPrimary, 2},
		pageDef{"/albums", "Albums", module.NavLibrary, 20},
		pageDef{"/songs", "Songs", module.NavLibrary, 30},
	))
	tt.Frame()
	if got := a.router.Path(); got != "/home" {
		t.Fatalf("starts at %q, want the first nav item", got)
	}
	if err := tt.Click("Songs"); err != nil {
		t.Fatal(err)
	}
	if got := a.router.Path(); got != "/songs" {
		t.Errorf("click: at %q, want /songs", got)
	}
	// Ctrl+N is the Nth primary link, Alt+N the Nth library link.
	tt.Key(ui.Ctrl, ui.Key2)
	if got := a.router.Path(); got != "/search" {
		t.Errorf("Ctrl+2: at %q, want /search", got)
	}
	tt.Key(ui.Alt, ui.Key1)
	if got := a.router.Path(); got != "/albums" {
		t.Errorf("Alt+1: at %q, want /albums", got)
	}
	tt.Key(ui.Ctrl, ui.Key9) // no ninth link: nothing happens
	if got := a.router.Path(); got != "/albums" {
		t.Errorf("Ctrl+9: at %q", got)
	}
	if !tt.HasText("page of Albums") {
		t.Errorf("page not shown: %q", tt.Texts())
	}
}

func TestFirstPaintRunsOnce(t *testing.T) {
	n := 0
	_, tt := newShellPaints(t, Options{}, &n, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}))
	if n != 1 {
		t.Fatalf("first paint reported %d times by the first frame, want once", n)
	}
	tt.Frame()
	tt.Frame()
	_ = tt.Click("Albums")
	tt.Frame()
	if n != 1 {
		t.Errorf("first paint reported %d times, want once", n)
	}
}

func TestStartPathFollowsTheSettingsWhenRegistered(t *testing.T) {
	s, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	pages := pageModule("a", pageDef{"/library/albums", "Albums", module.NavLibrary, 1}, pageDef{"/library/songs", "Songs", module.NavLibrary, 2})
	for _, tc := range []struct{ start, want string }{
		{"/library/songs", "/library/songs"}, // registered
		{"/", "/library/albums"},             // Home is not ported: the first link
		{"/charts", "/library/albums"},
	} {
		if _, err := s.Update(func(st *config.Settings) { st.Desktop.StartPage = tc.start }); err != nil {
			t.Fatal(err)
		}
		a := newApp(Options{Settings: s}, []module.Factory{pages})
		if a.start != tc.want {
			t.Errorf("start page %q opens %q, want %q", tc.start, a.start, tc.want)
		}
		a.env.Art.Close()
	}
}

func TestSidebarToggleAndCustomChord(t *testing.T) {
	s, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The user's binding replaces the default chord of a shortcut.
	if _, err := s.Update(func(st *config.Settings) { st.Shortcuts = config.Shortcuts{"nav.songs": "Ctrl+K"} }); err != nil {
		t.Fatal(err)
	}
	a, tt := newShell(t, Options{Settings: s}, pageModule("a",
		pageDef{"/albums", "Albums", module.NavPrimary, 1},
		pageDef{"/songs", "Songs", module.NavPrimary, 2},
	))
	tt.Frame()
	tt.Key(ui.Ctrl, ui.Key2)
	if a.router.Path() != "/albums" {
		t.Errorf("the replaced default chord still navigates: %q", a.router.Path())
	}
	tt.Key(ui.Ctrl, ui.KeyK)
	if a.router.Path() != "/songs" {
		t.Errorf("the user's chord does not navigate: %q", a.router.Path())
	}
	tt.Key(ui.Ctrl, ui.KeyB)
	if tt.HasText("Sona") {
		t.Error("Ctrl+B left the sidebar shown")
	}
	tt.Key(ui.Ctrl, ui.KeyB)
	if !tt.HasText("Sona") {
		t.Error("Ctrl+B again did not bring the sidebar back")
	}
}

func TestSlotsBuildWhenModulesHaveThem(t *testing.T) {
	open := false
	f := func(*module.Env) module.Module {
		return module.Module{
			ID: "slots",
			Slots: module.Slots{
				Bottom:        func(c *ui.Context) { ui.Text(c, "bottom bar") },
				Aside:         func(c *ui.Context) { ui.Text(c, "aside") },
				SidebarFooter: func(c *ui.Context) { ui.Text(c, "sidebar footer") },
				TopBarSearch:  func(c *ui.Context) { ui.Text(c, "search field") },
				Overlay:       &module.Overlay{Open: func() bool { return open }, Build: func(c *ui.Context) { ui.Text(c, "overlay") }},
			},
			Routes: []module.Route{{Pattern: "/p", Build: func(c *ui.Context, _ *ui.Route) { ui.Text(c, "page") }}},
			Nav:    []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "P" }, To: "/p"}},
		}
	}
	_, tt := newShell(t, Options{}, f)
	tt.Frame()
	for _, w := range []string{"bottom bar", "aside", "sidebar footer", "search field", "page"} {
		if !tt.HasText(w) {
			t.Errorf("%q not shown: %q", w, tt.Texts())
		}
	}
	if tt.HasText("overlay") {
		t.Error("a closed overlay shows")
	}
	open = true
	tt.Frame()
	if !tt.HasText("overlay") {
		t.Error("an open overlay does not show")
	}
}

func TestParseChord(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want chord
		ok   bool
	}{
		{"Space", chord{0, ui.KeySpace}, true},
		{"Ctrl+K", chord{ui.Ctrl, ui.KeyK}, true},
		{"Shift+ArrowRight", chord{ui.Shift, ui.KeyRight}, true},
		{"Alt+1", chord{ui.Alt, ui.Key1}, true},
		{"[", chord{0, ui.KeyBracketLeft}, true},
		{"Ctrl+Shift+L", chord{ui.Ctrl | ui.Shift, ui.KeyL}, true},
		{"Ctrl+Ctrl+K", chord{}, false},
		{"Hyper+K", chord{}, false},
		{"", chord{}, false},
	} {
		got, ok := parseChord(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseChord(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
	if _, ok := parseBinding("none"); !ok {
		t.Error(`"none" is a binding (no keys)`)
	}
	if c, ok := parseBinding("Ctrl+K Alt+K"); !ok || len(c) != 2 {
		t.Errorf("two chords: %v %v", c, ok)
	}
	if _, ok := parseBinding("Ctrl+K nonsense"); ok {
		t.Error("a binding with a bad chord parsed")
	}
}

func TestNavActive(t *testing.T) {
	item := module.NavItem{To: "/library"}
	for _, tc := range []struct {
		path  string
		exact bool
		want  bool
	}{
		{"/library", false, true},
		{"/library/albums", false, true},
		{"/library/albums", true, false},
		{"/libraryx", false, false},
		{"/other", false, false},
	} {
		item.Exact = tc.exact
		if got := navActive(tc.path, item); got != tc.want {
			t.Errorf("navActive(%q, exact=%v) = %v", tc.path, tc.exact, got)
		}
	}
}

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

// A text field keeps the keys it types: a plain-letter shortcut does not
// fire from it, and does elsewhere; a Ctrl chord still reaches a global
// shortcut.
func TestShortcutsLeaveTextFieldsTheirKeys(t *testing.T) {
	var plain, chorded int
	text := ""
	f := func(*module.Env) module.Module {
		return module.Module{
			ID: "keys",
			Shortcuts: []module.Shortcut{
				{ID: "keys.plain", Keys: []string{"S"}, Run: func() { plain++ }},
				{ID: "keys.chord", Keys: []string{"Ctrl+J"}, Scope: "global", Run: func() { chorded++ }},
			},
			Routes: []module.Route{{Pattern: "/p", Build: func(c *ui.Context, _ *ui.Route) {
				ui.TextInput(c.Key("field"), &text).Label("Field")
				ui.Button(c, "Elsewhere")
			}}},
			Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "P" }, To: "/p"}},
		}
	}
	_, tt := newShell(t, Options{}, f)
	tt.Frame()
	tt.Key(0, ui.KeyS)
	if plain != 1 {
		t.Fatalf("S outside a text field ran the shortcut %d times", plain)
	}
	if err := tt.Click("Field"); err != nil {
		t.Fatal(err)
	}
	tt.TypeKey(0, ui.KeyS, "s")
	if plain != 1 || text != "s" {
		t.Errorf("typing S in a text field: shortcut ran %d times, text %q", plain, text)
	}
	tt.Key(ui.Ctrl, ui.KeyJ)
	if chorded != 1 {
		t.Errorf("Ctrl+J from a text field ran the global shortcut %d times", chorded)
	}
}

func TestNewAppRegistersTheInterfaceFont(t *testing.T) {
	a := newApp(Options{}, nil)
	t.Cleanup(a.env.Art.Close)
	for _, f := range ui.FontFamilies() {
		if f.Name == theme.FontFamily && f.Registered {
			return
		}
	}
	t.Errorf("%q is not a registered font family", theme.FontFamily)
}

// A closed layer takes no pointer press: a click on the page reaches it
// (an empty full-window wrapper over the page took every one).
func TestAClosedLayerDoesNotCoverThePage(t *testing.T) {
	open := false
	clicks := 0
	f := func(*module.Env) module.Module {
		return module.Module{
			ID:    "layered",
			Slots: module.Slots{Layer: func(c *ui.Context) { ui.Text(c, "layer") }, LayerOpen: func() bool { return open }},
			Routes: []module.Route{{Pattern: "/p", Build: func(c *ui.Context, _ *ui.Route) {
				if ui.Button(c, "Press me").Clicked() {
					clicks++
				}
			}}},
			Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "P" }, To: "/p"}},
		}
	}
	_, tt := newShell(t, Options{}, f)
	tt.Frame()
	if err := tt.Click("Press me"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("clicks = %d with the layer closed, want 1", clicks)
	}
	open = true
	tt.Frame()
	if !tt.HasText("layer") {
		t.Error("the open layer is not built")
	}
}

// A first start opens the window on the first-run setup's route; every other
// unmatched path still lands on the start page.
func TestFirstRunOpensTheSetupRoute(t *testing.T) {
	var opened int
	f := func(*module.Env) module.Module {
		return module.Module{
			ID: "setup",
			Routes: []module.Route{
				{Pattern: services.OnboardingRoute, Build: func(c *ui.Context, _ *ui.Route) { opened++ }},
				{Pattern: "/p", Build: func(c *ui.Context, _ *ui.Route) { ui.Text(c, "page") }},
			},
			Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "P" }, To: "/p"}},
		}
	}
	a, tt := newShell(t, Options{FirstRun: true}, f)
	tt.Frame()
	if opened == 0 || a.start != "/p" {
		t.Fatalf("setup route built %d times, start page %q", opened, a.start)
	}
	if got := a.router.Path(); got != services.OnboardingRoute {
		t.Fatalf("path %q, want the setup route (the module redirects)", got)
	}
}

// Every feature's copy of the settings follows Go: a setting Go changes
// behind the cache (a scrobbler connected, private listening from the tray)
// reaches the cache on settings:changed, so the next save, which writes the
// whole settings, does not put the old value back.
func TestSettingsChangedKeepsTheSharedSettingsCurrent(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Now: h.Clock.Now}, nil)
	t.Cleanup(a.env.Art.Close)
	a.connect()
	if r := store.UseSettings(a.env.Queries, h.Core); !r.HasData {
		h.Main.Drain()
	}
	store.Set(a.env.Queries, store.KeySettings, h.Settings.Get()) // the cache holds the settings as they were

	if _, err := h.Settings.Update(func(s *config.Settings) {
		s.Scrobbling.LastFM = config.ScrobbleAccount{Enabled: true, Username: "listener"}
	}); err != nil {
		t.Fatal(err)
	}
	h.Bus.Emit(services.EventSettingsChanged, h.Settings.Get())
	h.Main.Drain()
	if got, _ := store.Peek[config.Settings](a.env.Queries, store.KeySettings); got.Scrobbling.LastFM.Username != "listener" {
		t.Fatalf("the cached settings did not follow: %+v", got.Scrobbling)
	}

	kit.UpdateSettings(a.env.Queries, h.Core, func(s *config.Settings) { s.ReduceEffects = true })
	deadline := time.Now().Add(10 * time.Second)
	for !h.Settings.Get().ReduceEffects && time.Now().Before(deadline) {
		h.Main.Drain()
		time.Sleep(5 * time.Millisecond)
	}
	got := h.Settings.Get()
	if !got.ReduceEffects {
		t.Fatal("the save never landed")
	}
	if got.Scrobbling.LastFM.Username != "listener" || !got.Scrobbling.LastFM.Enabled {
		t.Fatalf("the save put the old scrobbler back: %+v", got.Scrobbling)
	}
}
