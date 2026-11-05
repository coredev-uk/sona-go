package onboarding

import (
	"context"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The fakes of everything the setup asks of Go beyond the settings (which
// are the real SystemService's, saved to the Env's temporary store): the
// scrobblers would read the machine's keyring, the CDM would download.

type fakeScrobbler struct {
	mu        sync.Mutex
	status    services.ScrobbleStatus
	connects  int
	cancelled int
	tokens    []string
	err       error
}

func (f *fakeScrobbler) Status() services.ScrobbleStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeScrobbler) ConnectLastFM(context.Context) (services.ScrobbleStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connects++
	return f.status, f.err
}

func (f *fakeScrobbler) CancelConnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelled++
}

func (f *fakeScrobbler) ConnectListenBrainz(_ context.Context, token string) (services.ScrobbleStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append(f.tokens, token)
	if f.err != nil {
		return f.status, f.err
	}
	f.status.ListenBrainz.Connected = true
	return f.status, nil
}

func (f *fakeScrobbler) counts() (connects, cancelled int, tokens []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connects, f.cancelled, append([]string(nil), f.tokens...)
}

type fakeIntegrations struct{ status services.IntegrationsStatus }

func (f *fakeIntegrations) Status() services.IntegrationsStatus { return f.status }

type fakeTranslations struct{ apple services.AppleLanguages }

func (f *fakeTranslations) Languages(float64) services.AppleLanguages { return f.apple }

type fakeCDM struct {
	mu      sync.Mutex
	retries int
	release chan struct{} // when set, Retry waits for it
}

func (f *fakeCDM) Retry() error {
	f.mu.Lock()
	f.retries++
	wait := f.release
	f.mu.Unlock()
	if wait != nil {
		<-wait
	}
	return nil
}

func (f *fakeCDM) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.retries
}

// linuxCaps is a Linux desktop with everything the setup can offer.
func linuxCaps() services.Capabilities {
	return services.Capabilities{
		OS: "linux",
		Playback: services.PlaybackCapabilities{
			Available: true, Engine: "cdm", Volume: true, SoundCheck: true, Crossfade: true, PrivateListening: true,
		},
		Notifications: true, Tray: true, Autostart: true, SleepInhibit: true, PowerState: true, DiscordPresence: true,
	}
}

// bareCaps is a platform with nothing to offer.
func bareCaps() services.Capabilities { return services.Capabilities{OS: "windows"} }

func cdmStatus(patch func(*services.CDMStatus)) services.CDMStatus {
	s := services.CDMStatus{State: "ready", Version: "4.10.3112.0"}
	if patch != nil {
		patch(&s)
	}
	return s
}

// rig is the setup layer over the Env of a test, on a window w wide.
type rig struct {
	t   *testing.T
	w   float32
	h   *uitest.Harness
	o   *onboarding
	tt  *uitest.Session
	sc  *fakeScrobbler
	in  *fakeIntegrations
	tr  *fakeTranslations
	cdm *fakeCDM

	behind func(*ui.Context) // more of the app behind the setup, when a test wants it

	saved  int               // settings:changed events seen: every save makes one
	events []config.Settings // the settings those events carried
}

func newRig(t *testing.T, w, h int, opts ...uitest.Option) *rig {
	t.Helper()
	hn := uitest.New(t, opts...)
	r := &rig{t: t, w: float32(w), h: hn, sc: &fakeScrobbler{}, in: &fakeIntegrations{}, tr: &fakeTranslations{}, cdm: &fakeCDM{}}
	kit.SetToastPoster(hn.Main.Post)
	t.Cleanup(kit.CancelConfirm)
	r.o = newOnboarding(hn.Env, account.NewDeps(hn.Env), r.sc, r.in, r.tr, r.cdm)
	r.o.init()
	store.On(hn.Env.Bus, services.EventSettingsChanged, func(s config.Settings) { r.saved++; r.events = append(r.events, s) })
	q := hn.Env.Queries
	store.Set(q, store.KeyCapabilities, linuxCaps())
	store.Set(q, store.KeyCDM, cdmStatus(nil))
	store.Set(q, store.KeyPrivate, services.PrivateState{Source: "setting"})
	store.Set(q, store.KeyAuth, services.AuthStatus{State: "signed_out"})
	r.tt = hn.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Background(theme.C("ground")).Children(func() {
			ui.Text(c, "The app behind the setup")
			if r.behind != nil {
				r.behind(c)
			}
		})
		r.o.layer(c)
		kit.OverlayLayer(c)
	}, w, h)
	return r
}

// open opens the setup at a step, as a /welcome link or Settings do.
func (r *rig) open(step string) {
	r.t.Helper()
	r.h.Env.Surfaces.OpenSetup(step)
	r.tt.Drain()
}

func (r *rig) step() string { return r.h.Env.Surfaces.SetupStep() }

func (r *rig) click(text string) {
	r.t.Helper()
	if err := r.tt.Click(text); err != nil {
		r.t.Fatal(err)
	}
	r.tt.Drain()
}

// clickSwitch toggles the switch at the end of the row named text: the name
// is a plain text, the switch sits at the sheet's right edge on the same line.
func (r *rig) clickSwitch(text string) {
	r.t.Helper()
	at, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no row %q; texts %q", text, r.tt.Texts())
	}
	// The switch is centred on the row's two lines (the label's 20 and the hint's 18).
	r.tt.ClickAt(r.w/2+panelWidth/2-24-16, at.Y+19)
	r.tt.Drain()
}

func (r *rig) key(mods ui.Modifiers, k ui.Key) {
	r.tt.Key(mods, k)
	r.tt.Drain()
}

// tabTo presses Tab until the element named label has the focus.
func (r *rig) tabTo(label string) {
	r.t.Helper()
	for range 60 {
		if r.tt.Focused(label) {
			return
		}
		r.key(0, ui.KeyTab)
	}
	r.t.Fatalf("Tab never reached %q", label)
}

func (r *rig) hasAll(texts ...string) bool {
	for _, s := range texts {
		if !r.tt.HasText(s) {
			return false
		}
	}
	return true
}

func (r *rig) lacks(texts ...string) bool {
	for _, s := range texts {
		if r.tt.HasText(s) {
			return false
		}
	}
	return true
}

// sawSettings reports whether a "settings:changed" so far carried settings
// that match.
func (r *rig) sawSettings(match func(config.Settings) bool) bool {
	for _, s := range r.events {
		if match(s) {
			return true
		}
	}
	return false
}

func update(r *rig, change func(*config.Settings)) {
	r.t.Helper()
	if _, err := r.h.Settings.Update(change); err != nil {
		r.t.Fatal(err)
	}
}

func TestStepsGoThroughEveryStepInOrderAndStayOnTheLast(t *testing.T) {
	seen := []string{"welcome"}
	for range 8 {
		seen = append(seen, stepAfter(seen[len(seen)-1]))
	}
	want := []string{"welcome", "account", "playback", "connections", "appearance", "desktop", "done", "done", "done"}
	if len(seen) != len(want) {
		t.Fatalf("steps %q", seen)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("steps %q, want %q", seen, want)
		}
	}
	if stepBefore("account") != "welcome" || stepBefore("welcome") != "welcome" {
		t.Errorf("stepBefore: %q, %q", stepBefore("account"), stepBefore("welcome"))
	}
	names := map[string]string{
		"welcome": "Welcome", "account": "Apple Music", "playback": "Playback", "connections": "Connections",
		"appearance": "Appearance", "desktop": "Desktop", "done": "Done",
	}
	for step, name := range names {
		if got := stepName(step); got != name {
			t.Errorf("stepName(%q) = %q, want %q", step, got, name)
		}
	}
}

func TestParseStepStartsAtTheWelcomeForAMissingOrUnknownStep(t *testing.T) {
	for in, want := range map[string]string{
		"": "welcome", "listening": "welcome", "appearance": "appearance", "done": "done", "Desktop": "welcome",
	} {
		if got := parseStep(in); got != want {
			t.Errorf("parseStep(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWelcomeRouteOpensTheSetupAtItsStepAndRedirects(t *testing.T) {
	h := uitest.New(t)
	router := ui.NewRouter("/")
	h.Env.Nav = router
	o := newOnboarding(h.Env, account.NewDeps(h.Env), &fakeScrobbler{}, &fakeIntegrations{}, &fakeTranslations{}, &fakeCDM{})
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			router.View(c, func(rt *ui.Route) {
				if rt.Match(welcomePath) {
					o.welcome(c, rt)
				}
			})
		})
	}, 400, 300)
	tt.Frame()
	if h.Env.Surfaces.SetupOpen() {
		t.Fatal("the setup opened on /")
	}
	router.Push("/welcome?step=desktop")
	tt.Frame()
	if !h.Env.Surfaces.SetupOpen() || h.Env.Surfaces.SetupStep() != "desktop" {
		t.Errorf("setup open %v at %q, want open at desktop", h.Env.Surfaces.SetupOpen(), h.Env.Surfaces.SetupStep())
	}
	if router.Path() != "/" {
		t.Errorf("the route stayed at %q, want it replaced by /", router.Path())
	}
	h.Env.Surfaces.CloseSetup()
	router.Push("/welcome?step=listening")
	tt.Frame()
	if h.Env.Surfaces.SetupStep() != "welcome" {
		t.Errorf("an unknown step opened at %q", h.Env.Surfaces.SetupStep())
	}
}

func TestModuleIsTheSetupLayerOverTheWholeWindow(t *testing.T) {
	h := uitest.New(t)
	h.Env.Nav = ui.NewRouter("/")
	m := New(h.Env)
	if m.ID != "onboarding" || len(m.Routes) != 1 || m.Routes[0].Pattern != "/welcome" || m.Slots.Layer == nil || m.Slots.LayerOpen == nil {
		t.Fatalf("module %+v", m)
	}
	if m.Slots.LayerOpen() {
		t.Error("the layer is open before anything asked")
	}
	h.Env.Surfaces.OpenSetup("")
	if !m.Slots.LayerOpen() {
		t.Error("the layer is closed after OpenSetup")
	}
}

func TestWidevineViewReadsTheCapabilitiesFirstThenTheDownload(t *testing.T) {
	linux, bare := linuxCaps(), bareCaps()
	st := func(patch func(*services.CDMStatus)) *services.CDMStatus { s := cdmStatus(patch); return &s }
	native := linux
	native.Playback.Engine = "native"
	for _, c := range []struct {
		name   string
		caps   *services.Capabilities
		status *services.CDMStatus
		want   widevineState
	}{
		{"no capabilities", nil, st(nil), viewLoading},
		{"no engine", &bare, nil, viewUnavailable},
		{"no engine, an idle status", &bare, st(func(s *services.CDMStatus) { s.State = "" }), viewUnavailable},
		{"an engine without a CDM", &native, nil, viewNotNeeded},
		{"no status yet", &linux, nil, viewLoading},
		{"idle", &linux, st(func(s *services.CDMStatus) { s.State, s.Version = "idle", "" }), viewWaiting},
		{"downloading", &linux, st(func(s *services.CDMStatus) { s.State = "downloading" }), viewDownloading},
		{"ready", &linux, st(nil), viewReady},
		{"failed", &linux, st(func(s *services.CDMStatus) { s.State, s.Code = "failed", "network" }), viewFailed},
		{"no CDM for this computer", &linux, st(func(s *services.CDMStatus) { s.State, s.Code = "failed", "unsupported_platform" }), viewUnsupported},
	} {
		if got := widevineView(c.caps, c.status); got != c.want {
			t.Errorf("%s: view %d, want %d", c.name, got, c.want)
		}
	}
}

func TestWidevineShortStates(t *testing.T) {
	st := func(patch func(*services.CDMStatus)) *services.CDMStatus { s := cdmStatus(patch); return &s }
	for _, c := range []struct {
		view   widevineState
		status *services.CDMStatus
		want   string
	}{
		{viewLoading, nil, ""},
		{viewNotNeeded, nil, ""},
		{viewUnavailable, nil, "Not available here"},
		{viewUnsupported, nil, "Not available here"},
		{viewWaiting, nil, "Starting…"},
		{viewDownloading, st(func(s *services.CDMStatus) { s.Received, s.Total = 25, 100 }), "Downloading, 25%"},
		{viewDownloading, st(nil), "Downloading…"},
		{viewReady, st(nil), "Ready, version 4.10.3112.0"},
		{viewReady, st(func(s *services.CDMStatus) { s.Version = "" }), "Ready"},
		{viewFailed, st(func(s *services.CDMStatus) { s.Code = "network" }), "Couldn't download"},
		{viewFailed, st(func(s *services.CDMStatus) { s.Code = "load_failed" }), "Couldn't install"},
	} {
		if got := widevineShort(c.view, c.status); got != c.want {
			t.Errorf("view %d: %q, want %q", c.view, got, c.want)
		}
	}
}

func TestLayerIsAModalNamedByItsStepWithItsTitleFocused(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("playback")
	if !r.hasAll("Set up Sona", "3 of 7", "Playback", "What plays your music and how it sounds.", "Skip setup", "Back", "Continue") {
		t.Fatalf("the layer shows %q", r.tt.Texts())
	}
	// Every step's name shows under its segment.
	if !r.hasAll("Welcome", "Apple Music", "Connections", "Appearance", "Desktop", "Done") {
		t.Errorf("the strip lacks a step's name: %q", r.tt.Texts())
	}
	if !r.tt.Focused("Playback") {
		t.Error("the step's title does not have the focus")
	}
	// A press on the scrim does nothing: the setup stays.
	r.tt.ClickAt(5, 5)
	r.tt.Drain()
	if !r.h.Env.Surfaces.SetupOpen() {
		t.Error("a press outside the sheet closed the setup")
	}
}

func TestLayerBuildsNothingWhileClosed(t *testing.T) {
	r := newRig(t, 900, 800)
	r.tt.Drain()
	if r.tt.HasText("Set up Sona") || r.o.shown != "" {
		t.Errorf("the closed layer shows %q", r.tt.Texts())
	}
}

func TestGetStartedGoesOnAndBackGoesBack(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("welcome")
	if r.tt.HasText("Back") {
		t.Error("the welcome has a Back")
	}
	r.click("Get started")
	if r.step() != "account" {
		t.Fatalf("Get started went to %q", r.step())
	}
	r.h.Env.Surfaces.SetSetupStep("appearance")
	r.tt.Drain()
	r.click("Back")
	if r.step() != "connections" {
		t.Fatalf("Back went to %q", r.step())
	}
	if !r.tt.Focused("Connections") {
		t.Error("the new step's title does not have the focus")
	}
	r.click("Continue")
	if r.step() != "appearance" {
		t.Fatalf("Continue went to %q", r.step())
	}
}

func TestStepStripGoesToAnyStepOneTabStopMovedByArrows(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("account")
	// Shift+Tab from the title enters the strip.
	r.key(ui.Shift, ui.KeyTab)
	if !r.tt.Focused("Welcome") {
		t.Fatalf("Shift+Tab did not reach the strip; texts %q", r.tt.Texts())
	}
	r.key(0, ui.KeyRight)
	if r.tt.Focused("Welcome") {
		t.Errorf("→ did not leave the first segment")
	}
	r.key(0, ui.KeyEnd)
	if !r.tt.Focused("Done") {
		t.Errorf("End did not reach the last segment")
	}
	r.key(0, ui.KeyHome)
	if !r.tt.Focused("Welcome") {
		t.Errorf("Home did not reach the first segment")
	}
	// Moving the focus goes nowhere; Enter goes to the step.
	if r.step() != "account" {
		t.Fatalf("the arrows changed the step to %q", r.step())
	}
	r.key(0, ui.KeyEnd)
	r.key(0, ui.KeyEnter)
	if r.step() != "done" {
		t.Errorf("Enter on the last segment went to %q", r.step())
	}
	// Any segment is a button to its step (ahead or back).
	r.click("Desktop")
	if r.step() != "desktop" {
		t.Errorf("the Desktop segment went to %q", r.step())
	}
}

func TestSkipSetupLeavesAtOnceAndMarksItDone(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("account")
	if r.h.Settings.Get().Onboarding.Completed {
		t.Fatal("a fresh profile is already completed")
	}
	r.click("Skip setup")
	if r.h.Env.Surfaces.SetupOpen() {
		t.Fatal("Skip setup left the setup open")
	}
	r.tt.Until("the setting saved", func() bool { return r.h.Settings.Get().Onboarding.Completed })
	if r.tt.HasText("Set up Sona") {
		t.Error("the layer is still drawn after leaving")
	}
}

func TestEscapeAsksBeforeSkippingTheSetup(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("playback")
	r.key(0, ui.KeyEscape)
	if !r.hasAll("Skip the rest of the setup?", "What you've chosen so far is kept. Settings → General runs the setup again.") {
		t.Fatalf("Escape asked nothing; texts %q", r.tt.Texts())
	}
	r.click("Cancel")
	if !r.h.Env.Surfaces.SetupOpen() || r.tt.HasText("Skip the rest of the setup?") {
		t.Fatalf("Cancel did not keep the setup open (open %v)", r.h.Env.Surfaces.SetupOpen())
	}
	if r.h.Settings.Get().Onboarding.Completed {
		t.Error("Cancel saved the setup as done")
	}

	r.key(0, ui.KeyEscape)
	if !r.tt.HasText("Skip the rest of the setup?") {
		t.Fatal("the second Escape asked nothing")
	}
	// The confirming button has the focus: Enter confirms.
	r.key(0, ui.KeyEnter)
	if r.h.Env.Surfaces.SetupOpen() {
		t.Fatal("confirming left the setup open")
	}
	r.tt.Until("the setting saved", func() bool { return r.h.Settings.Get().Onboarding.Completed })
}

func TestEscapeOnTheDoneStepLeavesWithoutAsking(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("done")
	r.key(0, ui.KeyEscape)
	if r.tt.HasText("Skip the rest of the setup?") {
		t.Error("Escape on Done asked")
	}
	if r.h.Env.Surfaces.SetupOpen() {
		t.Error("Escape on Done left the setup open")
	}
}

func TestEveryOpeningStartsAtTheWelcomeUnlessAStepIsAsked(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("done")
	r.h.Env.Surfaces.CloseSetup() // as Done does
	r.tt.Drain()
	r.h.Env.Surfaces.OpenSetup("")
	r.tt.Drain()
	if !r.tt.HasText("Welcome to Sona") {
		t.Errorf("reopened somewhere else: %q", r.tt.Texts())
	}
	// Reopening while open only jumps to the step.
	r.h.Env.Surfaces.OpenSetup("connections")
	r.tt.Drain()
	if r.step() != "connections" || !r.tt.HasText("Optional. Keep a record of what you play, or show it on Discord.") {
		t.Errorf("did not jump to connections: %q", r.step())
	}
}

func TestASignInAskedForWhileOpenGoesToTheAppleMusicStep(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("appearance")
	r.h.Env.Surfaces.OpenAccount()
	r.tt.Drain()
	if r.h.Env.Surfaces.AccountOpen() {
		t.Error("the account dialog opened behind the setup")
	}
	if r.step() != "account" || !r.tt.HasText("Signing in brings your library") {
		t.Errorf("step %q; texts %q", r.step(), r.tt.Texts())
	}
}

func TestWelcomeSaysWhatWholeSongsNeedAndWhereEachStands(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
		s.State, s.Version, s.Received, s.Total = "downloading", "", 25, 100
	}))
	r.open("welcome")
	if !r.hasAll("What whole songs need", "Next step", "Downloading, 25%", "Widevine",
		"A subscription, signed in to Sona. Browsing works without one.") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	// Only English is on offer: no Language.
	if r.tt.HasText("Automatic follows your Apple Music account and your system.") {
		t.Error("the language is offered with English alone")
	}

	store.Set(r.h.Env.Queries, store.KeyAuth, services.AuthStatus{State: "signed_in"})
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(nil))
	r.tt.Drain()
	if !r.hasAll("Signed in", "Ready, version 4.10.3112.0") || r.tt.HasText("Next step") {
		t.Errorf("signed in and ready: %q", r.tt.Texts())
	}

	// Playback without a CDM: no Widevine row.
	native := linuxCaps()
	native.Playback.Engine = "native"
	store.Set(r.h.Env.Queries, store.KeyCapabilities, native)
	r.tt.Drain()
	if r.tt.HasText("Widevine") {
		t.Error("Widevine is listed where the engine needs none")
	}
}

func TestWelcomeOffersTheLanguageOnceThereIsAnotherOne(t *testing.T) {
	r := newRig(t, 900, 800)
	r.tr.apple = services.AppleLanguages{Mapped: 1, Locales: []services.AppleLanguage{{Tag: "fr-FR", Coverage: 0.95}}}
	r.open("welcome")
	r.tt.Until("the language", r.tt.Shows("Language", "Automatic follows your Apple Music account and your system."))

	// Choosing one saves it, and once Go has it every query is fetched again.
	r.o.setLanguage("de")
	r.tt.Until("the language saved", func() bool { return r.h.Settings.Get().Language == "de" })
	r.tt.Until("the queries invalidated", func() bool { return !r.o.languageAsked })
}

func TestAppleMusicStepOffersTheSignInAndLetsYouCarryOnWithoutIt(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("account")
	r.tt.Until("the form", r.tt.Shows("Not now", "Sign in"))
	r.click("Not now")
	if r.step() != "playback" {
		t.Errorf("Not now went to %q", r.step())
	}
}

func TestAppleMusicStepSaysYoureSignedInInsteadOfTheForm(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyAuth, services.AuthStatus{State: "signed_in"})
	r.open("account")
	if !r.hasAll("Signed in to Apple Music", "Your library syncs in the background while you carry on.", "Continue") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Not now") {
		t.Error("Not now beside a signed-in account")
	}
	store.Set(r.h.Env.Queries, store.KeyAuth, services.AuthStatus{State: "validating"})
	r.tt.Drain()
	if !r.tt.HasText("Checking your sign-in…") {
		t.Error("validating does not say it is checking")
	}
}

func TestPlaybackShowsTheDownloadsProgressOnItsBar(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
		s.State, s.Version, s.Received, s.Total = "downloading", "", 42, 100
	}))
	r.open("playback")
	if !r.hasAll("Downloading Widevine…", "Songs play once it's ready. The download carries on if you continue.",
		"42% · 42 byte of 100 byte") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	// Its size not known yet: starting.
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) { s.State, s.Version = "downloading", "" }))
	r.tt.Drain()
	if !r.tt.HasText("Starting…") {
		t.Errorf("an unknown size: %q", r.tt.Texts())
	}
}

func TestPlaybackWaitingAndReady(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) { s.State, s.Version = "idle", "" }))
	r.open("playback")
	if !r.tt.HasText("Starting Widevine's download…") {
		t.Errorf("waiting: %q", r.tt.Texts())
	}
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(nil))
	r.tt.Drain()
	if !r.tt.HasText("Widevine 4.10.3112.0 is ready") {
		t.Errorf("ready: %q", r.tt.Texts())
	}
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) { s.Version = "" }))
	r.tt.Drain()
	if !r.tt.HasText("Widevine is ready") {
		t.Errorf("ready, no version: %q", r.tt.Texts())
	}
}

func TestPlaybackSaysWhyTheDownloadFailedAndRetriesItKeepingTheFocusOnTheState(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
		s.State, s.Version, s.Code = "failed", "", "network"
	}))
	r.cdm.release = make(chan struct{})
	r.open("playback")
	if !r.hasAll("Couldn't download",
		"Sona couldn't reach Google's download server. Check your internet connection. Sona also tries again by itself later.",
		"Try again") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	r.click("Try again")
	r.tt.Until("the retry", func() bool { return r.cdm.count() == 1 })
	if !r.tt.HasText("Retrying…") || r.tt.HasText("Try again") {
		t.Errorf("while retrying: %q", r.tt.Texts())
	}
	if !r.tt.Focused("Couldn't download") {
		t.Error("the focus did not move to the state")
	}
	close(r.cdm.release)
	r.tt.Until("the retry done", r.tt.Shows("Try again"))
	if r.cdm.count() != 1 {
		t.Errorf("Retry was called %d times", r.cdm.count())
	}
}

func TestPlaybackOnAComputerGoogleMakesNoWidevineFor(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
		s.State, s.Version, s.Code = "failed", "", "unsupported_platform"
	}))
	r.open("playback")
	if !r.tt.HasText("Playback isn't available on this computer") || r.tt.HasText("Try again") {
		t.Errorf("texts %q", r.tt.Texts())
	}
}

func TestPlaybackOnAPlatformWithoutAnEngineOffersNothingElse(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCapabilities, bareCaps())
	r.open("playback")
	if !r.hasAll("Playback isn't available on this platform yet", "Browsing Apple Music and your library works as usual.") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	if !r.lacks("Audio quality", "Sound Check", "Crossfade", "Private listening", "Widevine download") {
		t.Errorf("controls without playback: %q", r.tt.Texts())
	}
}

func TestPlaybackSavesQualitySoundCheckCrossfadeAndPrivateListening(t *testing.T) {
	r := newRig(t, 900, 1100)
	r.open("playback")
	r.tt.Until("the controls", r.tt.Shows("Audio quality", "High quality", "Data saver", "Sound Check", "Crossfade", "Private listening"))
	if got := r.h.Settings.Get().AudioQuality; got != 256 {
		t.Fatalf("quality starts at %d", got)
	}
	r.click("Data saver")
	r.tt.Until("the quality saved", func() bool { return r.h.Settings.Get().AudioQuality == 64 })

	r.clickSwitch("Sound Check")
	r.tt.Until("Sound Check saved", func() bool { return r.h.Settings.Get().Audio.SoundCheck })

	if r.tt.HasText("Length") {
		t.Fatal("the crossfade length shows while crossfade is off")
	}
	r.clickSwitch("Crossfade")
	r.tt.Until("crossfade saved", func() bool { return r.h.Settings.Get().Crossfade.Enabled })
	r.tt.Until("the length", r.tt.Shows("Length", "6 s"))

	r.clickSwitch("Private listening")
	r.tt.Until("private listening on", func() bool {
		p, ok := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate)
		return ok && p.On
	})
}

func TestCrossfadeLengthIsSavedWhenTheKeyMovesIt(t *testing.T) {
	r := newRig(t, 900, 800)
	update(r, func(s *config.Settings) { s.Crossfade.Enabled, s.Crossfade.Seconds = true, 6 })
	r.open("playback")
	r.tt.Until("the length", r.tt.Shows("Length", "6 s"))
	r.tabTo("Length")
	r.key(0, ui.KeyRight)
	r.tt.Until("the length saved", func() bool { return r.h.Settings.Get().Crossfade.Seconds == 7 })
	r.tt.Until("the label follows", r.tt.Shows("7 s"))
}

func TestPrivateListeningIsFixedWhenTheEnvironmentForcesIt(t *testing.T) {
	r := newRig(t, 900, 1100)
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "environment"})
	r.open("playback")
	r.tt.Until("the hint", r.tt.Shows("On because Sona was started with SONA_PRIVATE=1."))
	r.clickSwitch("Private listening")
	if p, _ := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate); !p.On {
		t.Error("a forced private listening was switched off")
	}
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "development"})
	r.tt.Drain()
	if !r.tt.HasText("On in development builds; switching it here lasts until Sona quits.") {
		t.Errorf("development: %q", r.tt.Texts())
	}
}

func TestConnectionsOffersLastFMListenBrainzAndDiscord(t *testing.T) {
	r := newRig(t, 900, 800)
	r.sc.status = services.ScrobbleStatus{LastFM: services.ScrobblerStatus{Available: true}, ListenBrainz: services.ScrobblerStatus{Available: true}}
	r.in.status = services.IntegrationsStatus{Discord: services.DiscordStatus{Configured: true}}
	r.open("connections")
	r.tt.Until("the rows", r.tt.Shows("Last.fm", "ListenBrainz", "Connect Last.fm", "Connect ListenBrainz", "Discord status",
		"Adds each song you play to your Last.fm profile.", "Adds each song you play to your public ListenBrainz history."))

	r.click("Connect Last.fm")
	r.tt.Until("the approval asked", func() bool { n, _, _ := r.sc.counts(); return n == 1 })

	// ListenBrainz: the token form, Escape closes it and the focus goes back to Connect.
	r.click("Connect ListenBrainz")
	if !r.tt.HasText("User token") {
		t.Fatalf("no token form: %q", r.tt.Texts())
	}
	r.key(0, ui.KeyEscape)
	if r.tt.HasText("User token") || !r.tt.Focused("Connect ListenBrainz") {
		t.Errorf("the form stayed or the focus is lost; texts %q", r.tt.Texts())
	}
	if !r.h.Env.Surfaces.SetupOpen() {
		t.Error("Escape in the form closed the setup")
	}

	// A token connects it, and the row says so.
	r.click("Connect ListenBrainz")
	r.tt.Type("secret-token")
	r.key(0, ui.KeyEnter)
	r.tt.Until("the token checked", func() bool { _, _, tokens := r.sc.counts(); return len(tokens) == 1 })
	if _, _, tokens := r.sc.counts(); tokens[0] != "secret-token" {
		t.Errorf("the token sent was %q", tokens[0])
	}
	r.tt.Until("connected", func() bool { return r.tt.HasText("Connected") && !r.tt.HasText("User token") })
}

func TestConnectionsStateLines(t *testing.T) {
	r := newRig(t, 900, 800)
	update(r, func(s *config.Settings) {
		s.Scrobbling.LastFM.Username = "ada"
		s.Scrobbling.ListenBrainz.Username = ""
	})
	r.sc.status = services.ScrobbleStatus{
		LastFM:       services.ScrobblerStatus{Available: true, Connected: true},
		ListenBrainz: services.ScrobblerStatus{Available: true, Connected: true},
	}
	r.open("connections")
	r.tt.Until("connected", r.tt.Shows("Scrobbling as ada.", "Scrobbling."))
	if r.tt.HasText("Connect Last.fm") {
		t.Error("a connected service offers Connect")
	}

	// Needs reconnecting, then a build without Last.fm.
	r.sc.status = services.ScrobbleStatus{
		LastFM: services.ScrobblerStatus{Available: false, NeedsReconnect: true, Connected: true},
	}
	store.Set(r.h.Env.Queries, keyScrobble, r.sc.status)
	r.tt.Drain()
	if !r.hasAll("Reconnect Last.fm", "This build of Sona can't connect to Last.fm.") {
		t.Errorf("texts %q", r.tt.Texts())
	}

	// Last.fm waiting for the browser.
	r.sc.status = services.ScrobbleStatus{LastFM: services.ScrobblerStatus{Available: true, Connecting: true}}
	store.Set(r.h.Env.Queries, keyScrobble, r.sc.status)
	r.tt.Drain()
	if !r.hasAll("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself.", "Cancel") {
		t.Fatalf("waiting: %q", r.tt.Texts())
	}
	r.click("Cancel")
	r.tt.Until("cancelled", func() bool { _, n, _ := r.sc.counts(); return n == 1 })
}

func TestConnectionsDiscordAndPrivateListening(t *testing.T) {
	r := newRig(t, 900, 800)
	r.in.status = services.IntegrationsStatus{Discord: services.DiscordStatus{Configured: false}}
	r.open("connections")
	r.tt.Until("not configured", r.tt.Shows("Discord status", "Not available in this build of Sona."))
	if r.tt.HasText("Nothing is scrobbled") {
		t.Error("the private note shows while private listening is off")
	}

	store.Set(r.h.Env.Queries, keyIntegrations, services.IntegrationsStatus{Discord: services.DiscordStatus{Configured: true}})
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "setting"})
	r.tt.Drain()
	if !r.tt.HasText("Nothing is scrobbled or shown on Discord while private listening is on.") {
		t.Errorf("private on: %q", r.tt.Texts())
	}
	r.clickSwitch("Discord status")
	r.tt.Until("Discord saved", func() bool { return r.h.Settings.Get().Integrations.DiscordPresence })

	// Not on this platform at all: no row.
	caps := linuxCaps()
	caps.DiscordPresence = false
	store.Set(r.h.Env.Queries, store.KeyCapabilities, caps)
	r.tt.Drain()
	if r.tt.HasText("Discord status") {
		t.Error("Discord is offered where Sona can't reach it")
	}
}

func TestAppearanceSavesTheThemePickedFromItsPicturesAndReduceEffects(t *testing.T) {
	r := newRig(t, 900, 800)
	t.Cleanup(func() { theme.SetReduceEffects(false) })
	r.open("appearance")
	r.tt.Until("the picker", r.tt.Shows("Theme", "System", "Follows your desktop", "Light", "Always light", "Dark", "Always dark"))
	r.click("Dark")
	r.tt.Until("the theme saved", func() bool { return r.h.Settings.Get().Theme == "dark" })
	// The app behind changes at once.
	if !theme.Dark() {
		t.Error("the theme did not apply")
	}

	if r.h.Settings.Get().ReduceEffects {
		t.Fatal("reduce effects starts on")
	}
	r.tt.Until("the rows", r.tt.Shows("Reduce effects", "Motion artwork",
		"Plays the looping covers some albums have. Paused on battery power."))
	r.clickSwitch("Reduce effects")
	r.tt.Until("reduce effects saved", func() bool { return r.h.Settings.Get().ReduceEffects })
	r.tt.Until("motion off", r.tt.Shows("Off while Reduce effects is on."))

	// Motion artwork cannot be turned on while effects are reduced.
	before := r.h.Settings.Get().MotionArtwork.Enabled
	r.clickSwitch("Motion artwork")
	r.tt.Drain()
	if r.h.Settings.Get().MotionArtwork.Enabled != before {
		t.Error("motion artwork changed while reduce effects is on")
	}
}

func TestAppearanceWithoutPowerStateDoesNotPromiseToPause(t *testing.T) {
	r := newRig(t, 900, 800)
	caps := linuxCaps()
	caps.PowerState = false
	store.Set(r.h.Env.Queries, store.KeyCapabilities, caps)
	r.open("appearance")
	if !r.tt.HasText("Plays the looping covers some albums have.") || r.tt.HasText("Paused on battery power.") {
		t.Errorf("texts %q", r.tt.Texts())
	}
}

func TestDesktopSavesStartingAtLoginAndWhatClosingTheWindowDoes(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("desktop")
	r.tt.Until("the controls", r.tt.Shows("When you close the window", "Keep playing", "Quit Sona",
		"Start Sona when you log in", "Track notifications", "Stay awake while playing"))
	if r.tt.HasText("Start in the tray") {
		t.Error("Start in the tray shows while starting at login is off")
	}
	// The Env has no autostart entry to write, so DesktopService puts the
	// setting back after saving it: what the setup saved is the event.
	r.clickSwitch("Start Sona when you log in")
	r.tt.Until("autostart saved", func() bool {
		return r.sawSettings(func(s config.Settings) bool { return s.Desktop.Autostart })
	})
	r.click("Quit Sona")
	r.tt.Until("close action saved", func() bool { return r.h.Settings.Get().Desktop.CloseAction == "quit" })
	r.clickSwitch("Track notifications")
	r.tt.Until("notifications saved", func() bool { return !r.h.Settings.Get().Integrations.Notifications })
}

func TestDesktopStartInTheTrayShowsWithAutostartAndSaves(t *testing.T) {
	r := newRig(t, 900, 800)
	// Starting at login is on in the cache (the Env cannot keep it on: it has no autostart entry).
	s := r.h.Settings.Get()
	s.Desktop.Autostart, s.Desktop.StartHidden = true, true
	store.Set(r.h.Env.Queries, store.KeySettings, s)
	r.open("desktop")
	r.tt.Until("the row", r.tt.Shows("Start in the tray", "At login, Sona starts without a window, ready for your media keys."))
	r.clickSwitch("Start in the tray")
	r.tt.Until("start in the tray saved", func() bool {
		return r.sawSettings(func(s config.Settings) bool { return !s.Desktop.StartHidden })
	})
}

func TestDesktopNamesWhatThePlatformCannotDo(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCapabilities, bareCaps())
	r.open("desktop")
	r.tt.Until("the line", r.tt.Shows("Not on this platform yet: starting at login and track notifications.",
		"playing on in the background needs a system tray"))
	if !r.lacks("Quit Sona", "Start Sona when you log in", "Track notifications", "Stay awake while playing") {
		t.Errorf("controls the platform lacks: %q", r.tt.Texts())
	}
}

// A Mac source build has the tray, the sleep inhibitor and the rest, but
// not Sona.app: the line says that, not that the platform lacks them.
func TestDesktopSaysWhatOnlySonaAppCanDo(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCapabilities, services.Capabilities{
		OS: "darwin", Tray: true, SleepInhibit: true, NeedsAppBundle: true,
	})
	r.open("desktop")
	r.tt.Until("the line", r.tt.Shows("Only when Sona runs from Sona.app: starting at login and track notifications.",
		"Stay awake while playing", "Keep playing"))
	if !r.lacks("Not on this platform yet", "Start Sona when you log in", "Track notifications") {
		t.Errorf("a source build offers or blames the platform: %q", r.tt.Texts())
	}
}

// In Sona.app the login item is described as macOS has it.
func TestDesktopDescribesTheMacLoginItem(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCapabilities, services.Capabilities{
		OS: "darwin", Tray: true, Autostart: true, Notifications: true, PlainNotifications: true,
	})
	r.open("desktop")
	r.tt.Until("the row", r.tt.Shows("Start Sona when you log in", "Adds Sona to the login items in System Settings → General."))
	if r.tt.HasText("startup applications") || r.tt.HasText("Not on this platform yet") || r.tt.HasText("Only when Sona runs from Sona.app") {
		t.Errorf("texts %q", r.tt.Texts())
	}
}

func TestDoneMarksTheSetupDoneOnceAndListsWhatIsStillToDo(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyCDM, cdmStatus(func(s *services.CDMStatus) {
		s.State, s.Version, s.Received, s.Total = "downloading", "", 50, 100
	}))
	r.open("done")
	r.tt.Until("marked done", func() bool { return r.h.Settings.Get().Onboarding.Completed })
	if !r.hasAll("Setup is done", "Still to do", "Apple Music", "Widevine", "Sign in", "Go to Playback",
		"Not signed in. Signing in brings your library and plays whole songs.",
		"Downloading, 50%. Songs play once it's ready; it carries on in the background.") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	if r.tt.HasText("Skip setup") {
		t.Error("Skip setup on the last step")
	}
	r.click("Go to Playback")
	if r.step() != "playback" {
		t.Fatalf("Go to Playback went to %q", r.step())
	}
	r.h.Env.Surfaces.SetSetupStep("done")
	r.tt.Drain()
	r.click("Sign in")
	if r.step() != "account" {
		t.Fatalf("Sign in went to %q", r.step())
	}
}

func TestDoneButtonLeavesAndMarksTheSetupDoneExactlyOnce(t *testing.T) {
	r := newRig(t, 900, 800)
	r.open("done")
	r.tt.Until("marked done", func() bool { return r.h.Settings.Get().Onboarding.Completed })
	r.tt.Drain()
	// The strip has a "Done" too (the step): the button is on the foot's line, at the right.
	back, ok := r.tt.Find("Back")
	if !ok {
		t.Fatalf("no Back; texts %q", r.tt.Texts())
	}
	r.tt.ClickAt(r.w/2+panelWidth/2-24-24, back.Y+back.H/2)
	r.tt.Drain()
	if r.h.Env.Surfaces.SetupOpen() {
		t.Fatal("Done left the setup open")
	}
	r.tt.Drain()
	if r.saved != 1 {
		t.Errorf("settings saved %d times, want once", r.saved)
	}
}

func TestDoneSaysSonaIsReadyWhenNothingIsLeftAndDoesNotSaveAgain(t *testing.T) {
	r := newRig(t, 900, 800)
	update(r, func(s *config.Settings) { s.Onboarding.Completed = true })
	store.Set(r.h.Env.Queries, store.KeyAuth, services.AuthStatus{State: "signed_in"})
	r.open("done")
	if !r.hasAll("You're all set", "Ready to play", "Your library syncs in the background.") || r.tt.HasText("Still to do") {
		t.Fatalf("texts %q", r.tt.Texts())
	}
	r.key(0, ui.KeyEscape)
	r.tt.Drain()
	if r.saved != 0 {
		t.Errorf("an already completed setup saved %d times", r.saved)
	}
}

func TestDoneWithoutPlaybackSaysSo(t *testing.T) {
	r := newRig(t, 900, 800)
	store.Set(r.h.Env.Queries, store.KeyAuth, services.AuthStatus{State: "signed_in"})
	store.Set(r.h.Env.Queries, store.KeyCapabilities, bareCaps())
	r.open("done")
	if !r.tt.HasText("Playback isn't available here yet") {
		t.Errorf("texts %q", r.tt.Texts())
	}
}

// The setup's two settings-backed steps show a failed read as the block with
// its retry, never the controls; the settings arriving replace it.
func TestStepsShowAFailedSettingsReadWithTryAgainAndRecover(t *testing.T) {
	for _, c := range []struct {
		step     string
		controls []string
	}{
		{"playback", []string{"Audio quality", "High quality", "Sound Check"}},
		{"desktop", []string{"When you close the window", "Keep playing", "Quit Sona"}},
	} {
		t.Run(c.step, func(t *testing.T) {
			r := newRig(t, 900, 1100)
			r.h.Env.Core = nil // the settings read fails
			r.open(c.step)
			r.tt.Until("the error block", r.tt.Shows("Couldn't read Sona's settings", "Try again"))
			for _, w := range c.controls {
				if r.tt.HasText(w) {
					t.Errorf("the failed read still shows %q: %q", w, r.tt.Texts())
				}
			}
			r.click("Try again") // asks again; the service is still not there
			r.tt.Until("the error block again", r.tt.Shows("Couldn't read Sona's settings", "Try again"))

			store.Set(r.h.Env.Queries, store.KeySettings, r.h.Settings.Get()) // the settings arrive
			r.tt.Until("the controls", r.tt.Shows(c.controls...))
			if r.tt.HasText("Couldn't read Sona's settings") {
				t.Errorf("the error stayed after the settings arrived: %q", r.tt.Texts())
			}
		})
	}
}

// Whatever opened the setup gets the focus back when it closes.
func TestFocusReturnsToWhatOpenedTheSetupWhenItCloses(t *testing.T) {
	r := newRig(t, 900, 800)
	r.behind = func(c *ui.Context) {
		if kit.Button(c.Key("opener"), "Set up Sona again", kit.ButtonOpts{}).Clicked() {
			r.h.Env.Surfaces.OpenSetup("")
		}
	}
	r.tt.Drain()
	r.click("Set up Sona again")
	if !r.h.Env.Surfaces.SetupOpen() || !r.tt.HasText("Skip setup") {
		t.Fatalf("the button did not open the setup: %q", r.tt.Texts())
	}
	if r.tt.Focused("Set up Sona again") {
		t.Fatal("the focus stayed behind the modal setup")
	}
	r.click("Skip setup")
	if r.h.Env.Surfaces.SetupOpen() {
		t.Fatal("Skip setup left the setup open")
	}
	if !r.tt.Focused("Set up Sona again") {
		t.Errorf("the focus did not return to the opener; texts %q", r.tt.Texts())
	}
}
