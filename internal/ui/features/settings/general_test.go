package settings

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeUpdates is the update check without GitHub.
type fakeUpdates struct {
	mu         sync.Mutex
	status     services.UpdateStatus
	checkErr   error
	openErr    error
	checks     int
	opens      int
	installs   int
	installErr error
	onOpen     func()
}

func (f *fakeUpdates) Status() services.UpdateStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeUpdates) Check(context.Context) (services.UpdateStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks++
	return f.status, f.checkErr
}

func (f *fakeUpdates) OpenRelease() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opens++
	if f.onOpen != nil {
		f.onOpen()
	}
	return f.openErr
}

func (f *fakeUpdates) Install(context.Context) (services.UpdateStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installs++
	return f.status, f.installErr
}

func (f *fakeUpdates) count() (checks, opens int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checks, f.opens
}

type fakeDesktop struct {
	mu     sync.Mutex
	resets int
}

func (f *fakeDesktop) ResetWindowSize() { f.mu.Lock(); f.resets++; f.mu.Unlock() }
func (f *fakeDesktop) count() int       { f.mu.Lock(); defer f.mu.Unlock(); return f.resets }

type fakeLanguages struct{ l services.AppleLanguages }

func (f fakeLanguages) Languages(float64) services.AppleLanguages { return f.l }

// generalRig is the page on General over fakes for everything that would
// touch the machine (updates, the window, Apple's locales), with the
// toasts drawn, capabilities of a full Linux desktop and settings the test
// may change first.
type generalRig struct {
	*rig
	up *fakeUpdates
	dt *fakeDesktop
}

func newGeneralRig(t *testing.T, caps services.Capabilities, prepare func(*config.Settings)) *generalRig {
	t.Helper()
	hn := uitest.New(t)
	hn.Env.HasRoute = func(p string) bool { return p == "/library/albums" }
	if prepare != nil {
		cur := hn.Settings.Get()
		prepare(&cur)
		if _, err := hn.Settings.Update(func(s *config.Settings) { *s = cur }); err != nil {
			t.Fatal(err)
		}
	}
	r := &rig{t: t, h: hn, router: ui.NewRouter(settingsPath)}
	hn.Env.Nav = r.router
	r.s = newSettings(hn.Env, services.Service[services.SystemService](hn.Core), services.Service[services.PlatformService](hn.Core),
		fakeScrobbler{}, services.Service[services.IntegrationsService](hn.Core),
		services.Service[services.CDMService](hn.Core), nil)
	up := &fakeUpdates{status: services.UpdateStatus{Current: "1.0.0", Checkable: true}}
	dt := &fakeDesktop{}
	r.s.updates = up
	stateOf[desktopSeam](r.s, "seam:desktop").api = dt
	stateOf[languagesSeam](r.s, "seam:translations").api = fakeLanguages{}
	r.s.init()
	followShell(hn)
	store.Set(hn.Env.Queries, store.KeyUpdates, up.status) // the updates module's mirror
	store.Set(hn.Env.Queries, store.KeyCapabilities, caps)
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
			kit.Toaster(c)
		})
	}, 1132, 800)
	g := &generalRig{rig: r, up: up, dt: dt}
	// Every first fetch has landed (a later Set would be overwritten).
	r.tt.Until("General", r.tt.Shows("Starting Sona", "Updates", "Check now"))
	return g
}

func (g *generalRig) click(label string) {
	g.t.Helper()
	if err := g.tt.Click(label); err != nil {
		g.t.Fatal(err)
	}
	g.tt.Drain()
}

// focus lands the keyboard on a row's control, as Find a setting does (the
// row's title is the first text of that name, so clicking it hits the
// title).
func (g *generalRig) focus(id string) {
	g.t.Helper()
	g.s.focusSetting(id)
	g.tt.Drain()
	g.tt.Drain()
}

// toggle flips a row's switch with Space.
func (g *generalRig) toggle(id string) {
	g.t.Helper()
	g.focus(id)
	g.tt.Key(0, ui.KeySpace)
	g.tt.Drain()
}

// choose opens a row's select with Enter and clicks an option.
func (g *generalRig) choose(id, option string) {
	g.t.Helper()
	g.focus(id)
	g.tt.Key(0, ui.KeyEnter)
	g.tt.Drain()
	g.click(option)
}

func (g *generalRig) saved() config.Settings { return g.h.Settings.Get() }

func (g *generalRig) until(what string, ok func() bool) {
	g.t.Helper()
	g.tt.Until(what, ok)
}

func TestGeneralShowsEveryGroupAndRowInOrder(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), func(s *config.Settings) { s.Desktop.Autostart = false })
	g.tt.Until("rows", g.tt.Shows("Starting Sona", "Window", "While music plays", "Updates", "Start Sona when you log in",
		"Open on", "First-run setup", "When the window is closed", "Title bar", "Window size", "Track notifications",
		"Stay awake while playing", "Check for updates when Sona starts"))
	if g.tt.HasText("Start in the tray") {
		t.Error("Start in the tray shows with autostart off")
	}
	// The language is English and chosen ("en" in the harness): offered.
	if !g.tt.HasText("Language") || !g.s.languageRow() {
		t.Error("the Language row is not offered while a language is chosen")
	}
}

func TestLanguageRowIsHiddenWithEnglishAloneAndAutomatic(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), func(s *config.Settings) { s.Language = "" })
	g.tt.Until("rows", g.tt.Shows("Starting Sona"))
	if g.tt.HasText("Automatic follows your Apple Music") || g.s.languageRow() {
		t.Error("Language is offered with English alone and Automatic")
	}
}

func TestChangingLanguageSavesAndRefetchesEveryQuery(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), func(s *config.Settings) { s.Language = "fr" })
	g.tt.Until("language", g.tt.Shows("Français"))
	fetches := 0
	var mu sync.Mutex
	probe := func() int {
		mu.Lock()
		defer mu.Unlock()
		return fetches
	}
	use := func() {
		store.Use(g.h.Env.Queries, store.Key{"probe"}, func(context.Context) (int, error) {
			mu.Lock()
			defer mu.Unlock()
			fetches++
			return fetches, nil
		}, store.Options{Stale: store.Forever})
	}
	use()
	g.until("probe fetched", func() bool { use(); return probe() == 1 })
	g.choose("language", "English")
	g.until("saved", func() bool { return g.saved().Language == "en" })
	g.until("refetched in the new language", func() bool { use(); return probe() >= 2 })
	// The same language is a no-op.
	g.choose("language", "English")
	g.tt.Drain()
	if g.saved().Language != "en" {
		t.Errorf("language %q", g.saved().Language)
	}
	// Automatic stores the empty string.
	g.choose("language", "Automatic")
	g.until("automatic", func() bool { return g.saved().Language == "" })
}

func TestOpenOnOffersTheRegisteredPagesAndTheChoiceIsWhereSonaOpens(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), func(s *config.Settings) { s.Desktop.LastRoute = "/charts"; s.Desktop.ResumeLastRoute = true })
	*stateOf[func(string) bool](g.s, "seam:hasRoute") = func(p string) bool { return p == "/library/albums" || p == "/library/songs" }
	g.tt.Drain()
	g.choose("start-page", "Songs")
	g.until("start page", func() bool {
		d := g.saved().Desktop
		return d.StartPage == "/library/songs" && !d.ResumeLastRoute && d.LastRoute == ""
	})
	// What Sona opens on at the next start (the shell reads StartPage; the
	// shell's own test covers that it opens there).
	if got := g.saved().Desktop.StartRoute(); got != "/library/songs" {
		t.Errorf("the next start opens %q", got)
	}
	// A page no module registered is not offered; the last page always is.
	g.tt.Key(0, ui.KeyEnter)
	g.tt.Drain()
	if g.tt.HasText("Charts") {
		t.Error("an unregistered page is offered")
	}
	if !g.tt.HasText("The page you were last on") {
		t.Error("the last page is not offered")
	}
	g.tt.Key(0, ui.KeyEscape)
	g.tt.Drain()
}

func TestOpenOnOffersThePageYouWereLastOnAndSavesItAsResume(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), func(s *config.Settings) {
		s.Desktop.StartPage = "/library/songs"
		s.Desktop.LastRoute = "/library/songs"
	})
	*stateOf[func(string) bool](g.s, "seam:hasRoute") = func(p string) bool { return p == "/library/albums" || p == "/library/songs" }
	g.tt.Drain()
	g.until("page and summary", g.tt.Shows("Songs", "Opens on Songs"))
	g.choose("start-page", "The page you were last on")
	g.until("resume saved", func() bool {
		d := g.saved().Desktop
		return d.ResumeLastRoute && d.StartPage == "/library/songs"
	})
	g.until("trigger and summary", g.tt.Shows("The page you were last on", "Opens where you left off"))
	if g.tt.HasText("Opens on Songs") {
		t.Error("the summary still names a page")
	}
	// Choosing a page again turns the memory off.
	g.choose("start-page", "Albums")
	g.until("page saved", func() bool {
		d := g.saved().Desktop
		return !d.ResumeLastRoute && d.StartPage == "/library/albums" && d.LastRoute == ""
	})
	g.until("summary", g.tt.Shows("Opens on Albums"))
}

func TestOpenOnShowsThePageTheShellOpens(t *testing.T) {
	// The saved "/" is registered by no module here: the shell opens the
	// first registered page, and the select and the summary say so.
	g := newGeneralRig(t, linuxCaps(), nil)
	g.until("trigger and summary", g.tt.Shows("Albums", "Opens on Albums"))
	if g.tt.HasText("Opens where you left off") {
		t.Error("the summary names route memory with it off")
	}
	*stateOf[func(string) bool](g.s, "seam:hasRoute") = func(p string) bool { return p == "/charts" || p == "/library/songs" }
	if route, label := g.s.openOn(config.Desktop{StartPage: "/library/songs"}); route != "/library/songs" || label != "Songs" {
		t.Errorf("saved page offered: %q %q", route, label)
	}
	if route, label := g.s.openOn(config.Desktop{StartPage: "/"}); route != "/charts" || label != "Charts" {
		t.Errorf("fallback: %q %q", route, label)
	}
	*stateOf[func(string) bool](g.s, "seam:hasRoute") = func(string) bool { return false }
	if route, _ := g.s.openOn(config.Desktop{StartPage: "/"}); route != "" {
		t.Errorf("nothing registered: %q", route)
	}
}

func TestCloseActionSavesAndTheServiceReadsIt(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.choose("close-action", "Quit Sona")
	g.until("quit", func() bool { return g.saved().Desktop.CloseAction == config.CloseQuits })
	g.choose("close-action", "Keep playing in the background")
	g.until("background", func() bool { return g.saved().Desktop.CloseAction == config.CloseToBackground })
}

func TestCloseActionIsUnavailableWithoutATray(t *testing.T) {
	caps := linuxCaps()
	caps.Tray = false
	g := newGeneralRig(t, caps, nil)
	g.tt.Until("reason", g.tt.Shows("Sona quits: it can't stay in the system tray on Linux yet."))
}

func TestStartInTheTraySavesWhenAutostartIsOn(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	// The cached copy says autostart is on, as after turning it on.
	cur := g.saved()
	cur.Desktop.Autostart, cur.Desktop.StartHidden = true, true
	store.Set(g.h.Env.Queries, store.KeySettings, cur)
	g.tt.Until("row", g.tt.Shows("Start in the tray"))
	g.toggle("start-hidden")
	g.until("start hidden off", func() bool { return !g.saved().Desktop.StartHidden })
}

func TestAutostartIsRefusedWhereTheServiceHasNoBackend(t *testing.T) {
	// The harness's DesktopService has no autostart backend: turning it on
	// is refused and the setting returns to off (desktop_test.go).
	g := newGeneralRig(t, linuxCaps(), nil)
	g.toggle("autostart")
	g.until("reverted", func() bool {
		return !g.saved().Desktop.Autostart && !g.tt.HasText("Start in the tray")
	})
}

func TestAutostartIsAnUnavailableRowWithoutTheCapability(t *testing.T) {
	caps := linuxCaps()
	caps.Autostart, caps.Notifications, caps.SleepInhibit = false, false, false
	g := newGeneralRig(t, caps, nil)
	g.tt.Until("unavailable", g.tt.Shows("Start Sona when you log in", "Track notifications", "Stay awake while playing",
		"Not available on Linux yet."))
}

func TestTitleBarSelectSavesTheStyleAndSaysItAppliesFromTheNextStart(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.tt.Until("row", g.tt.Shows("Title bar", "Automatic leaves the title bar to your desktop, which Sona doesn't recognise."))
	if g.tt.HasText("Applies the next time Sona starts.") {
		t.Error("the hint shows before a change")
	}
	g.choose("title-bar", "The desktop's")
	g.until("saved", func() bool { return g.saved().TitleBar.Style == config.TitleBarDesktop })
	g.until("next-start hint", g.tt.Shows("The desktop's", "Applies the next time Sona starts."))
	g.choose("title-bar", "Sona's, without window buttons")
	g.until("saved again", func() bool { return g.saved().TitleBar.Style == config.TitleBarSonaNoButtons })
	// Back to the style the window was created with: nothing pending.
	g.choose("title-bar", "Automatic")
	g.until("auto", func() bool {
		return g.saved().TitleBar.Style == config.TitleBarAuto && !g.tt.HasText("Applies the next time Sona starts.")
	})
}

func TestTitleBarHintSaysWhatAutomaticPicksHere(t *testing.T) {
	caps := linuxCaps()
	caps.WindowControls.Automatic, caps.WindowControls.Desktop = "sona", "GNOME"
	g := newGeneralRig(t, caps, nil)
	g.tt.Until("hint", g.tt.Shows("Automatic uses Sona's title bar on GNOME, with the window buttons where your desktop puts them."))
	caps.WindowControls.Desktop = ""
	g2 := newGeneralRig(t, caps, nil)
	g2.tt.Until("hint", g2.tt.Shows("Automatic uses Sona's title bar, with the window buttons where your system puts them."))
	caps.WindowControls.Automatic, caps.WindowControls.Desktop = "desktop", "sway"
	g3 := newGeneralRig(t, caps, nil)
	g3.tt.Until("hint", g3.tt.Shows("Automatic leaves the title bar to sway."))
}

func TestTitleBarIsOnlyShownWhereTheWindowHasAFrame(t *testing.T) {
	for _, native := range []string{"inset", "none"} {
		caps := linuxCaps()
		caps.WindowControls.Native = native
		g := newGeneralRig(t, caps, nil)
		g.tt.Until("window group", g.tt.Shows("Window size"))
		if g.tt.HasText("Title bar") {
			t.Errorf("the Title bar row shows with Native %q", native)
		}
	}
}

func TestResetSizeCallsTheServiceAndNeverToastsOnSuccess(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), func(s *config.Settings) { s.Window = config.Window{Width: 1280, Height: 800, Maximised: true} })
	g.tt.Until("size", g.tt.Shows("Sona opens at the size you left it, maximised: 1280 × 800."))
	g.reveal("window-size")
	g.click("Reset size")
	g.until("reset", func() bool { return g.dt.count() == 1 })
	if g.tt.HasText("Couldn't reset the window") {
		t.Error("toast after a successful reset")
	}
}

func TestSetupRowOpensTheWelcomeStep(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.click("Set up Sona again")
	if !g.h.Env.Surfaces.SetupOpen() || g.h.Env.Surfaces.SetupStep() != store.SetupWelcome {
		t.Error("the setup did not open at Welcome")
	}
}

func TestNotificationsAndStayAwakeSave(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.toggle("track-notifications")
	g.until("notifications", func() bool { return !g.saved().Integrations.Notifications })
	g.toggle("inhibit-sleep")
	g.until("inhibit", func() bool { return !g.saved().Integrations.InhibitSleep })
}

func TestIntegrationsNotesSayWhenTheDesktopLacksTheService(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	store.Set(g.h.Env.Queries, keyIntegrations, services.IntegrationsStatus{Notifications: "unavailable", Inhibit: "unavailable"})
	g.until("notes", g.tt.Shows("No notification service is running on this desktop.",
		"Neither logind nor the desktop portal answered, so sleep can't be blocked."))
}

func TestUpdatesCheckSwitchSaves(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.toggle("updates-check")
	g.until("saved", func() bool { return !g.saved().Updates.Check })
}

func TestUpdateVerdicts(t *testing.T) {
	now := g0(time.Now())
	cases := []struct {
		name string
		st   services.UpdateStatus
		want []string
		not  []string
	}{
		{"dev", services.UpdateStatus{Current: "dev"}, []string{"Development build (dev): not checked for updates"}, []string{"Check now"}},
		{"dev no version", services.UpdateStatus{}, []string{"Development build (no version): not checked for updates"}, nil},
		{
			"available",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true},
			[]string{"Sona 1.1.0 is available", "Download the new package from the release page", "View release", "Check now"},
			nil,
		},
		{
			"appimage",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, AppImage: true},
			[]string{"Update this AppImage in place"},
			nil,
		},
		{
			"self-update",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, SelfUpdate: true},
			[]string{"Sona 1.1.0 is available", "Install it now and Sona restarts into it", "Install and restart"},
			nil,
		},
		{
			"installing",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, SelfUpdate: true, Installing: true, Downloaded: 1, Total: 4},
			[]string{"Downloading Sona 1.1.0…", "Downloading…"},
			[]string{"Install it now"},
		},
		{
			"install failed",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, SelfUpdate: true, InstallError: services.CodeUpdateSignature},
			[]string{"The update isn't signed with Sona's key"},
			nil,
		},
		{
			"no signing key",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, Disabled: services.UpdatesUnsigned},
			[]string{"This build has no update signing key", "View release"},
			[]string{"Install and restart"},
		},
		{
			"not writable",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.0.0", Checked: now.UnixMilli(), Disabled: services.UpdatesNotWritable},
			[]string{"Sona can't write where it is installed"},
			[]string{"Install and restart"},
		},
		{
			"error",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Error: services.CodeNetwork},
			[]string{"Couldn't check GitHub for updates"},
			[]string{"View release"},
		},
		{"never", services.UpdateStatus{Current: "1.0.0", Checkable: true}, []string{"Sona 1.0.0 · not checked yet"}, nil},
		{
			"up to date",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.0.0", Checked: now.Add(-5 * time.Minute).UnixMilli()},
			[]string{"Sona 1.0.0 is up to date", "Checked 5 min. ago."},
			[]string{"View release"},
		},
		{
			"no release",
			services.UpdateStatus{Current: "1.0.0", Checkable: true, Checked: now.UnixMilli()},
			[]string{"Sona 1.0.0 · no release published yet", "Checked just now."},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newGeneralRig(t, linuxCaps(), nil)
			g.s.clock = func() time.Time { return now }
			store.Set(g.h.Env.Queries, store.KeyUpdates, tc.st)
			g.until("verdict", g.tt.Shows(tc.want...))
			for _, n := range tc.not {
				if g.tt.HasText(n) {
					t.Errorf("shows %q", n)
				}
			}
		})
	}
}

// g0 is t unchanged (a named start for the verdict table).
func g0(t time.Time) time.Time { return t }

func TestCheckNowRunsTheCheckAndReplacesTheStatus(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.up.status = services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.2.0", Available: true}
	g.until("button", g.tt.Shows("Check now"))
	g.toEnd()
	g.click("Check now")
	g.until("new verdict", g.tt.Shows("Sona 1.2.0 is available"))
	if c, _ := g.up.count(); c != 1 {
		t.Errorf("checks %d", c)
	}
	if got, _ := store.Peek[services.UpdateStatus](g.h.Env.Queries, store.KeyUpdates); got.Latest != "1.2.0" {
		t.Errorf("cache %+v", got)
	}
}

func TestInstallAndRestartInstallsAndAFailureToasts(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	store.Set(g.h.Env.Queries, store.KeyUpdates, services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true, SelfUpdate: true})
	g.until("button", g.tt.Shows("Install and restart"))
	g.toEnd()
	g.click("Install and restart")
	g.until("installed", func() bool { g.up.mu.Lock(); defer g.up.mu.Unlock(); return g.up.installs == 1 })

	g.up.mu.Lock()
	g.up.installErr = errors.New("disk full")
	g.up.mu.Unlock()
	g.toEnd()
	g.click("Install and restart")
	g.until("toast", g.tt.Shows("Couldn't install the update"))
}

func TestTestVersionsSwitchSaves(t *testing.T) {
	caps := linuxCaps()
	caps.OS = "darwin"
	g := newGeneralRig(t, caps, nil)
	store.Set(g.h.Env.Queries, store.KeyUpdates, services.UpdateStatus{Current: "1.0.0", Checkable: true, SelfUpdate: true})
	g.until("row", g.tt.Shows("Test versions"))
	g.toggle("updates-prereleases")
	g.until("saved", func() bool { return g.saved().Updates.Prereleases })
}

func TestFailedCheckSaysWhy(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.up.checkErr = errors.New("boom")
	g.until("button", g.tt.Shows("Check now"))
	g.toEnd()
	g.click("Check now")
	g.until("failure", g.tt.Shows("Couldn't check: "))
}

func TestViewReleaseOpensThePageAndAFailureToasts(t *testing.T) {
	g := newGeneralRig(t, linuxCaps(), nil)
	g.up.onOpen = func() { _ = g.h.Env.OpenURL("https://github.com/coredev-uk/sona/releases/latest") }
	store.Set(g.h.Env.Queries, store.KeyUpdates, services.UpdateStatus{Current: "1.0.0", Checkable: true, Latest: "1.1.0", Available: true})
	g.until("button", g.tt.Shows("View release"))
	g.toEnd()
	g.click("View release")
	g.until("opened", func() bool { return len(g.h.Opened()) == 1 })

	g.up.mu.Lock()
	g.up.openErr = errors.New("no browser")
	g.up.mu.Unlock()
	g.toEnd()
	g.click("View release")
	g.until("toast", g.tt.Shows("Couldn't open the release"))
}
