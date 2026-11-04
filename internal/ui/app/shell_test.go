package app

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeNative is a window with a shown state.
type fakeNative struct {
	shown, focused, minimised bool
	closes                    int
}

func (f *fakeNative) Show()             { f.shown = true }
func (f *fakeNative) Hide()             { f.shown = false; f.focused = false }
func (f *fakeNative) Focus()            { f.focused = f.shown }
func (f *fakeNative) Close()            { f.closes++ }
func (f *fakeNative) Restore()          { f.minimised = false }
func (f *fakeNative) Minimize()         { f.minimised = true }
func (*fakeNative) ToggleMaximize()     {}
func (*fakeNative) Unmaximize()         {}
func (*fakeNative) SetSize(int, int)    {}
func (f *fakeNative) IsVisible() bool   { return f.shown }
func (f *fakeNative) IsFocused() bool   { return f.focused }
func (f *fakeNative) IsMinimized() bool { return f.minimised }

// windowsRig is a window control over fake windows: the main one shown,
// the mini player made on first use, quits counted.
type windowsRig struct {
	w          *Windows
	main, mini *fakeNative
	quits      atomic.Int32
	quitsSet   bool // the close action is Quit
	shownMini  []bool
}

func newWindowsRig(tray, withMini bool) *windowsRig {
	r := &windowsRig{main: &fakeNative{shown: true, focused: true}}
	r.w = &Windows{run: func(fn func()) { fn() }}
	r.w.quit = func() { r.quits.Add(1) }
	r.w.setTray(tray)
	hooks := windowHooks{
		closeQuits: func() bool { return r.quitsSet },
		miniShown:  func(on bool) { r.shownMini = append(r.shownMini, on) },
	}
	if withMini {
		hooks.newMini = func() native { r.mini = &fakeNative{}; return r.mini }
	}
	r.w.bind(r.main, nil, hooks)
	return r
}

// quitWait waits for Quit's goroutine.
func (r *windowsRig) quitWait(t *testing.T, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for r.quits.Load() < int32(want) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := r.quits.Load(); got != int32(want) {
		t.Fatalf("quits = %d, want %d", got, want)
	}
}

func TestClosingTheWindowKeepsPlayingInTheBackgroundOrQuits(t *testing.T) {
	r := newWindowsRig(true, false)
	r.w.closeWindow(r.main)
	if r.main.shown || r.w.quitting.Load() {
		t.Fatalf("background: shown %v quitting %v, want the window hidden and Sona running", r.main.shown, r.w.quitting.Load())
	}
	// The tray, a second launch or MPRIS Raise bring it back, focused.
	r.w.ShowMain()
	if !r.main.shown || !r.main.focused {
		t.Fatal("ShowMain did not bring the hidden window back")
	}

	r.quitsSet = true
	r.w.closeWindow(r.main)
	r.quitWait(t, 1)
	if !r.main.shown {
		t.Error("a quitting close hid the window first")
	}
}

func TestWithoutATrayClosingTheWindowQuits(t *testing.T) {
	r := newWindowsRig(false, false)
	r.w.closeWindow(r.main)
	r.quitWait(t, 1)
	r.w.Quit() // a second quit is the same quit
	if got := r.quits.Load(); got != 1 {
		t.Errorf("quits = %d after a second Quit", got)
	}
}

func TestMiniPlayerTakesTheMainWindowsPlaceAndGivesItBack(t *testing.T) {
	r := newWindowsRig(true, true)
	r.w.ToggleMini()
	if r.mini == nil || !r.mini.shown || !r.mini.focused || r.main.shown {
		t.Fatalf("after ToggleMini: mini %+v main shown %v", r.mini, r.main.shown)
	}
	if !r.w.MiniShown() {
		t.Error("MiniShown is false while it shows")
	}
	r.w.ToggleMini()
	if r.mini.shown || !r.main.shown || !r.main.focused {
		t.Fatalf("toggling again: mini shown %v main shown %v", r.mini.shown, r.main.shown)
	}

	// The expand button, or MPRIS Raise while the mini player shows.
	r.w.ToggleMini()
	first := r.mini
	r.w.ShowMain()
	if r.mini.shown || !r.main.shown {
		t.Fatal("ShowMain left the mini player up")
	}
	// Its close button goes to the background like the main window's.
	r.w.ToggleMini()
	if r.mini != first {
		t.Error("the mini player was made again instead of shown")
	}
	r.w.CloseMini()
	if r.mini.shown || r.main.shown || r.w.quitting.Load() {
		t.Fatalf("CloseMini to the background: mini %v main %v quitting %v", r.mini.shown, r.main.shown, r.w.quitting.Load())
	}
	want := []bool{true, false, true, false, true, false}
	if len(r.shownMini) != len(want) {
		t.Fatalf("mini shown changes %v, want %v", r.shownMini, want)
	}
	for i := range want {
		if r.shownMini[i] != want[i] {
			t.Fatalf("mini shown changes %v, want %v", r.shownMini, want)
		}
	}
}

func TestWithoutAMiniPlayerTheMainWindowStays(t *testing.T) {
	r := newWindowsRig(true, false)
	r.w.ToggleMini()
	if !r.main.shown || r.w.MiniShown() {
		t.Errorf("ToggleMini without a mini player: main shown %v", r.main.shown)
	}
}

func TestRouteForOpen(t *testing.T) {
	for _, tc := range []struct {
		t    services.OpenTarget
		want string
	}{
		{services.OpenTarget{Kind: "album", ID: "1440"}, "/album/1440"},
		{services.OpenTarget{Kind: "library-album", ID: "l.abc"}, "/album/l.abc"},
		{services.OpenTarget{Kind: "song", ID: "7", AlbumID: "1440"}, "/album/1440?song=7"},
		{services.OpenTarget{Kind: "song", ID: "7"}, "/song/7"},
		{services.OpenTarget{Kind: "artist", ID: "5"}, "/artist/5"},
		{services.OpenTarget{Kind: "playlist", ID: "pl.u-x"}, "/playlist/pl.u-x"},
		{services.OpenTarget{Kind: "library-playlist", ID: "p.y"}, "/playlist/p.y"},
		{services.OpenTarget{Kind: "music-video", ID: "9"}, "/music-video/9"},
		{services.OpenTarget{Kind: "station", ID: "ra.1"}, "/station/ra.1"},
		{services.OpenTarget{Kind: "curator", ID: "3"}, "/curator/3"},
		{services.OpenTarget{Kind: "podcast", ID: "3"}, ""},
		{services.OpenTarget{Kind: "album"}, ""},
	} {
		if got := routeForOpen(tc.t); got != tc.want {
			t.Errorf("routeForOpen(%+v) = %q, want %q", tc.t, got, tc.want)
		}
	}
}

func TestALinkOpensItsPageWhenAModuleShowsIt(t *testing.T) {
	bus := store.NewBus(discardLog())
	a, tt := newShell(t, Options{Bus: bus}, pageModule("a",
		pageDef{"/library/albums", "Albums", module.NavLibrary, 1},
		pageDef{"/album/{id}", "Album", module.NavFooter, 2},
	))
	tt.Frame()
	bus.Emit(services.EventAppOpen, services.OpenTarget{Kind: "song", ID: "7", AlbumID: "1440"})
	tt.Until("the album page", func() bool { return a.router.Location() == "/album/1440?song=7" })
	// No module shows artists: the link changes nothing.
	bus.Emit(services.EventAppOpen, services.OpenTarget{Kind: "artist", ID: "5"})
	tt.Frame()
	if loc := a.router.Location(); loc != "/album/1440?song=7" {
		t.Errorf("a link without a page moved the window to %q", loc)
	}
}

func TestResumeOpensTheLastPageAndRemembersTheNextOne(t *testing.T) {
	h := uitest.New(t)
	if _, err := h.Settings.Update(func(s *config.Settings) {
		s.Desktop.ResumeLastRoute, s.Desktop.LastRoute = true, "/b?x=1"
	}); err != nil {
		t.Fatal(err)
	}
	pages := pageModule("p", pageDef{"/a", "A", module.NavPrimary, 1}, pageDef{"/b", "B", module.NavPrimary, 2}, pageDef{"/c", "C", module.NavPrimary, 3})
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Now: h.Clock.Now}, []module.Factory{pages})
	t.Cleanup(a.env.Art.Close)
	a.connect()
	if loc := a.router.Location(); loc != "/b?x=1" {
		t.Fatalf("opened on %q, want the last page", loc)
	}
	tt := h.Tester(t, a.view, 1132, 800)
	tt.Frame()
	a.router.Push("/c")
	tt.Frame()
	// Passing through a page saves nothing; one that stays a second does.
	tt.Advance(routeSaveDelay - time.Millisecond)
	tt.Frame()
	if got := h.Settings.Get().Desktop.LastRoute; got != "/b?x=1" {
		t.Fatalf("saved %q before the page stayed a second", got)
	}
	tt.Advance(time.Millisecond)
	tt.Until("the route saved", func() bool { return h.Settings.Get().Desktop.LastRoute == "/c" })

	// A route no module shows any more opens the start page.
	if _, err := h.Settings.Update(func(s *config.Settings) { s.Desktop.LastRoute = "/gone" }); err != nil {
		t.Fatal(err)
	}
	b := newApp(Options{Settings: h.Settings}, []module.Factory{pages})
	t.Cleanup(b.env.Art.Close)
	if loc := b.router.Location(); loc != b.start {
		t.Errorf("a gone route opened %q, want the start page %q", loc, b.start)
	}
}

func TestTrayMenuFollowsPlaybackPrivateListeningAndTheMiniPlayer(t *testing.T) {
	var s trayState
	v := s.view()
	if v.now != "Not playing" || v.play != "Play" || v.hasItem || v.canNext || v.private != "" || v.mini != "Mini Player" {
		t.Fatalf("idle: %+v", v)
	}
	s.play = services.PlaybackState{State: "playing", Current: &services.QueueItem{Name: "Song", Artist: "Band"}, Controls: services.PlaybackControls{Next: true}}
	s.private = &services.PrivateState{On: true, Source: "environment"}
	s.mini = true
	v = s.view()
	if v.now != "Song – Band" || v.play != "Pause" || !v.hasItem || !v.canNext || v.canPrev {
		t.Errorf("playing: %+v", v)
	}
	if v.private != "Private listening (forced by SONA_PRIVATE=1)" || v.privateEnabled || !v.privateOn {
		t.Errorf("forced private listening: %q enabled %v on %v", v.private, v.privateEnabled, v.privateOn)
	}
	if v.mini != "Close Mini Player" {
		t.Errorf("mini item %q while it shows", v.mini)
	}
	s.private = &services.PrivateState{On: true, Source: "development"}
	if v := s.view(); v.private != "Private listening (development build)" || !v.privateEnabled {
		t.Errorf("development private listening: %q enabled %v", v.private, v.privateEnabled)
	}
}

func TestStatusBannerSaysWhatHoldsTheAccountUp(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Now: h.Clock.Now},
		[]module.Factory{pageModule("p", pageDef{"/a", "A", module.NavPrimary, 1})})
	t.Cleanup(a.env.Art.Close)
	a.connect()
	tt := h.Tester(t, a.view, 1132, 800)
	for _, tc := range []struct {
		st     services.AuthStatus
		text   string
		action string
	}{
		{services.AuthStatus{State: "signed_in", Offline: true}, "Offline. Showing your library from this computer; Sona reconnects by itself.", ""},
		{services.AuthStatus{State: "signed_out", Keyring: "unavailable"}, "Sona couldn't read your sign-in from your keyring, so you're signed out.", "Try again"},
		{services.AuthStatus{State: "signing_in", Keyring: "waiting"}, "Saving your sign-in in your keyring; unlock it if it asks.", ""},
		{services.AuthStatus{State: "error"}, "Apple Music isn't accepting Sona's requests.", "Diagnostics"},
		{services.AuthStatus{State: "needs_reauth"}, "Apple Music asked Sona to sign in again. Your library is still here.", "Sign in"},
	} {
		store.Set(a.env.Queries, store.KeyAuth, tc.st)
		tt.Until(tc.text, tt.Shows(tc.text))
		if tc.action != "" {
			if _, ok := tt.Find(tc.action); !ok {
				t.Errorf("%s: no %q", tc.st.State, tc.action)
			}
		}
	}
	if err := tt.Click("Sign in"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.env.Surfaces.AccountOpen() {
		t.Error("Sign in did not open the account dialog")
	}
	store.Set(a.env.Queries, store.KeyAuth, services.AuthStatus{State: "signed_in"})
	tt.Frame()
	if tt.HasText("Apple Music asked Sona to sign in again. Your library is still here.") {
		t.Error("the banner stays once the account is fine")
	}
}

// A language written right to left (the language shown, as Settings →
// General → Language chooses it) lays the whole window out from the right:
// the sidebar moves to the right edge.
func TestARightToLeftLanguageMirrorsTheWindow(t *testing.T) {
	_, tt := newShell(t, Options{}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}))
	tt.Frame()
	ltr, ok := tt.Find("Albums")
	if !ok {
		t.Fatal("no Albums link")
	}
	i18n.SetLocale("ar")
	t.Cleanup(func() { i18n.SetLocale("") })
	tt.Frame()
	rtl, _ := tt.Find("Albums")
	if ltr.X > 300 || rtl.X < 800 {
		t.Errorf("the sidebar link at x %v left to right and x %v right to left, want it moved to the right edge", ltr.X, rtl.X)
	}
}

// The pseudo locale (a development setting) shows as chosen: there are no
// Apple translations to look it up in.
func TestThePseudoLocaleSettingIsShown(t *testing.T) {
	h := uitest.New(t)
	if _, err := h.Settings.Update(func(s *config.Settings) { s.Language = "en-XA" }); err != nil {
		t.Fatal(err)
	}
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Now: h.Clock.Now},
		[]module.Factory{pageModule("p", pageDef{"/a", "Albums", module.NavLibrary, 1})})
	t.Cleanup(a.env.Art.Close)
	t.Cleanup(func() { i18n.SetLocale("") })
	a.connect()
	h.Main.Drain()
	if got := i18n.Locale(); got != "en-XA" {
		t.Fatalf("locale %q, want en-XA", got)
	}
}
