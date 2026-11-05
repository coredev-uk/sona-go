package settings

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/scrobble"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// connFake is a scripted scrobbler: nothing touches the keyring or network.
type connFake struct {
	mu       sync.Mutex
	status   services.ScrobbleStatus
	calls    []string
	lfmErr   error
	lbErr    error
	lfmNext  services.ScrobbleStatus
	lbNext   services.ScrobbleStatus
	hold     chan struct{} // ConnectLastFM waits for it when set
	discHold chan struct{} // Disconnect waits for it when set
	discErr  error
}

func (f *connFake) Status() services.ScrobbleStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *connFake) note(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *connFake) did() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *connFake) ConnectLastFM(context.Context) (services.ScrobbleStatus, error) {
	f.note("lastfm")
	f.mu.Lock()
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lfmNext, f.lfmErr
}

func (f *connFake) CancelConnect() {
	f.note("cancel")
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.hold != nil {
		f.lfmErr = context.Canceled
		close(f.hold)
		f.hold = nil
	}
}

func (f *connFake) ConnectListenBrainz(_ context.Context, token string) (services.ScrobbleStatus, error) {
	f.note("lb:" + token)
	return f.lbNext, f.lbErr
}

func (f *connFake) Disconnect(p string) (services.ScrobbleStatus, error) {
	f.note("disconnect:" + p)
	if f.discHold != nil {
		<-f.discHold
	}
	return services.ScrobbleStatus{Unencrypted: false}, f.discErr
}

type connInteg struct{ st services.IntegrationsStatus }

func (c connInteg) Status() services.IntegrationsStatus { return c.st }

type connPlat struct{ caps services.Capabilities }

func (c connPlat) Capabilities() services.Capabilities { return c.caps }

type connRig struct {
	*rig
	fake *connFake
	core *services.Core
}

func newConnRig(t *testing.T, fake *connFake, integ services.IntegrationsStatus, caps services.Capabilities) *connRig {
	t.Helper()
	hn := uitest.New(t)
	r := &rig{t: t, h: hn, router: ui.NewRouter(settingsPath + "?section=connections")}
	hn.Env.Nav = r.router
	store.Set(hn.Env.Queries, store.KeyCapabilities, caps)
	hn.Env.Shortcuts = func() []module.BoundShortcut { return nil }
	r.s = newSettings(hn.Env, services.Service[services.SystemService](hn.Core), connPlat{caps}, fake, connInteg{integ},
		services.Service[services.CDMService](hn.Core), services.Service[services.UpdatesService](hn.Core))
	r.s.init()
	followShell(hn)
	store.Set(hn.Env.Queries, keyScrobble, fake.Status())
	store.Set(hn.Env.Queries, keyIntegrations, integ)
	store.Set(hn.Env.Queries, store.KeyPrivate, services.PrivateState{Source: "setting"})
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
		})
		kit.ConfirmHost(c)
	}, 1132, 700)
	t.Cleanup(kit.CancelConfirm)
	return &connRig{rig: r, fake: fake, core: hn.Core}
}

var allCaps = services.Capabilities{DiscordPresence: true, OS: "linux"}

func (r *connRig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatalf("click %q: %v", label, err)
	}
}

func (r *connRig) saved() config.Settings {
	return services.Service[services.SystemService](r.core).Settings()
}

// setSaved stores settings through the service, as a connected account would
// have them.
func (r *connRig) setSaved(f func(*config.Settings)) {
	r.t.Helper()
	svc := services.Service[services.SystemService](r.core)
	s := svc.Settings()
	f(&s)
	if _, err := svc.UpdateSettings(s); err != nil {
		r.t.Fatal(err)
	}
	store.Set(r.h.Env.Queries, store.KeySettings, svc.Settings())
}

func TestConnectionsShowsTheScrobblingRows(t *testing.T) {
	r := newConnRig(t, &connFake{status: services.ScrobbleStatus{
		LastFM: services.ScrobblerStatus{Available: true}, ListenBrainz: services.ScrobblerStatus{Available: true},
	}}, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("rows", r.tt.Shows("Scrobbling", "Last.fm", "ListenBrainz", "Discord", "Discord status",
		"Adds each song you play to your Last.fm profile.", "Adds each song you play to your public ListenBrainz history."))
}

func TestLastFMWithoutAPIKeyCannotConnect(t *testing.T) {
	r := newConnRig(t, &connFake{}, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("notice", r.tt.Shows("This build of Sona has no Last.fm API key, so it can't connect to Last.fm."))
	_ = r.tt.Click("Connect Last.fm")
	r.tt.Drain()
	for _, c := range r.fake.did() {
		if c == "lastfm" {
			t.Fatal("Connect ran without an API key")
		}
	}
}

func TestLastFMConnectWaitsAndCancelShowsNothing(t *testing.T) {
	f := &connFake{status: services.ScrobbleStatus{LastFM: services.ScrobblerStatus{Available: true}, ListenBrainz: services.ScrobblerStatus{Available: true}}, hold: make(chan struct{})}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("Last.fm"))
	// Two Connect buttons: Last.fm's is first on the page.
	if err := r.tt.Click("Connect Last.fm"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("waiting panel", r.tt.Shows("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself."))
	r.click("Cancel")
	r.tt.Until("panel gone", func() bool {
		return !r.tt.HasText("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself.")
	})
	if r.tt.HasText("Couldn't connect Last.fm") {
		t.Fatal("a cancel toasted")
	}
	got := f.did()
	if len(got) < 2 || got[0] != "lastfm" || got[1] != "cancel" {
		t.Fatalf("calls %v", got)
	}
}

func TestLastFMConnectFailureEndsTheWait(t *testing.T) {
	f := &connFake{
		status: services.ScrobbleStatus{LastFM: services.ScrobblerStatus{Available: true}, ListenBrainz: services.ScrobblerStatus{Available: true}},
		lfmErr: scrobble.ErrExpired,
	}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("Last.fm"))
	r.click("Connect Last.fm")
	r.tt.Until("called", func() bool { return len(f.did()) == 1 })
	r.tt.Drain()
	if r.tt.HasText("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself.") {
		t.Fatal("still waiting after the failure")
	}
}

func TestErrorLines(t *testing.T) {
	cases := []struct {
		p    string
		err  error
		want string
	}{
		{services.ScrobblerLastFM, context.Canceled, ""},
		{services.ScrobblerListenBrainz, scrobble.ErrAuth, "ListenBrainz didn't accept that token. Copy it again from your ListenBrainz settings."},
		{services.ScrobblerLastFM, scrobble.ErrAuth, "Last.fm refused Sona's request. Try connecting again."},
		{services.ScrobblerLastFM, scrobble.ErrExpired, "The Last.fm approval expired before it was confirmed. Connect again to get a new one."},
		{services.ScrobblerListenBrainz, &url.Error{Op: "Get", URL: "x", Err: errors.New("no route")}, "Couldn't reach ListenBrainz. Check your connection and try again."},
		{services.ScrobblerLastFM, scrobble.ErrNotConfigured, "This build of Sona has no Last.fm API key, so it can't connect to Last.fm."},
	}
	for _, c := range cases {
		if got := errorLine(c.p, c.err); got != c.want {
			t.Errorf("errorLine(%s, %v) = %q, want %q", c.p, c.err, got, c.want)
		}
	}
	if got := errorLine(services.ScrobblerLastFM, errors.New("boom")); got == "" {
		t.Error("an unknown error has no sentence")
	}
}

func TestListenBrainzFormConnects(t *testing.T) {
	f := &connFake{
		status: services.ScrobbleStatus{LastFM: services.ScrobblerStatus{Available: true}, ListenBrainz: services.ScrobblerStatus{Available: true}},
		lbNext: services.ScrobbleStatus{ListenBrainz: services.ScrobblerStatus{Available: true, Connected: true}},
	}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("ListenBrainz"))
	r.setSaved(func(s *config.Settings) {})
	// ListenBrainz's Connect is the second Connect button; open via the label of
	// the row's only enabled one after Last.fm's is disabled by lack of a key.
	f.mu.Lock()
	f.status.LastFM.Available = false
	f.mu.Unlock()
	store.Set(r.h.Env.Queries, keyScrobble, f.Status())
	r.tt.Drain()
	r.click("Connect ListenBrainz")
	r.tt.Until("form", r.tt.Shows("User token", "your ListenBrainz settings"))
	if !r.tt.Focused("User token") {
		t.Error("the token field has no focus")
	}
	r.tt.Type("good ")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("connected", func() bool { return !r.tt.HasText("User token") })
	if got := f.did(); len(got) != 1 || got[0] != "lb:good " {
		t.Fatalf("calls %v", got)
	}
	if got := r.s.scrobbleStatus().Data; !got.ListenBrainz.Connected {
		t.Error("the status did not take the answer")
	}
}

func TestListenBrainzFormErrorBlankAndEscape(t *testing.T) {
	f := &connFake{
		status: services.ScrobbleStatus{ListenBrainz: services.ScrobblerStatus{Available: true}},
		lbErr:  scrobble.ErrAuth,
	}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("ListenBrainz"))
	r.click("Connect ListenBrainz")
	r.tt.Until("form", r.tt.Shows("User token"))
	// Blank: Connect (inside the form) does nothing.
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	if len(f.did()) != 0 {
		t.Fatal("a blank token was sent")
	}
	r.tt.Type("bad")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("alert", r.tt.Shows("ListenBrainz didn't accept that token. Copy it again from your ListenBrainz settings."))
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Until("closed", func() bool { return !r.tt.HasText("User token") })
	if r.s.state["scrobbling"].(*scrobbleUI).lb {
		t.Fatal("form still open")
	}
}

// Closing the token form (Cancel or Escape) takes its buttons away: the focus
// goes back to the row's Connect button, not to the page.
func TestListenBrainzFormClosingReturnsTheFocusToConnect(t *testing.T) {
	f := &connFake{status: services.ScrobbleStatus{ListenBrainz: services.ScrobblerStatus{Available: true}}}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("ListenBrainz"))
	for _, closeBy := range []string{"Cancel", "Escape"} {
		r.click("Connect ListenBrainz")
		r.tt.Until("form", r.tt.Shows("User token"))
		if r.tt.Focused("Connect ListenBrainz") {
			t.Fatalf("%s: the focus stayed on the button the form replaced", closeBy)
		}
		if closeBy == "Cancel" {
			r.click("Cancel")
		} else {
			r.tt.Key(0, ui.KeyEscape)
		}
		r.tt.Until("closed", func() bool { return !r.tt.HasText("User token") })
		r.tt.Drain()
		if !r.tt.Focused("Connect ListenBrainz") {
			t.Errorf("%s: the focus did not return to Connect ListenBrainz; texts %q", closeBy, r.tt.Texts())
		}
	}
}

func TestListenBrainzHintOpensTheTokenPage(t *testing.T) {
	f := &connFake{status: services.ScrobbleStatus{ListenBrainz: services.ScrobblerStatus{Available: true}}}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("ListenBrainz"))
	r.click("Connect ListenBrainz")
	r.tt.Until("form", r.tt.Shows("your ListenBrainz settings"))
	r.click("your ListenBrainz settings")
	if got := r.h.Opened(); len(got) != 1 || got[0] != "https://listenbrainz.org/settings/" {
		t.Fatalf("opened %v", got)
	}
}

func connectedFake(pending int) *connFake {
	return &connFake{status: services.ScrobbleStatus{
		LastFM:       services.ScrobblerStatus{Available: true, Connected: true, Pending: pending},
		ListenBrainz: services.ScrobblerStatus{Available: true},
	}}
}

func TestConnectedRowShowsUserAndPendingAndPausesWithTheSwitch(t *testing.T) {
	f := connectedFake(3)
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.setSaved(func(s *config.Settings) { s.Scrobbling.LastFM = config.ScrobbleAccount{Enabled: true, Username: "ann"} })
	r.tt.Until("line", r.tt.Shows("Scrobbling as ann. 3 songs waiting to send."))
	r.toggle("scrobble-lastfm")
	r.tt.Until("paused", func() bool { return !r.saved().Scrobbling.LastFM.Enabled })
	r.tt.Until("line", r.tt.Shows("Connected as ann, paused. Songs you play now aren't sent. 3 songs waiting to send."))
}

func TestDisconnectWithPendingAsksFirst(t *testing.T) {
	f := connectedFake(2)
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("Disconnect"))
	r.click("Disconnect Last.fm")
	r.tt.Until("dialog", r.tt.Shows("Disconnect Last.fm?", "2 songs you played haven't been sent yet. Sona discards them and removes its sign-in from your keyring."))
	if len(f.did()) != 0 {
		t.Fatal("disconnected before the answer")
	}
	r.click("Keep connected")
	r.tt.Drain()
	if len(f.did()) != 0 {
		t.Fatal("cancel disconnected")
	}
	r.click("Disconnect Last.fm")
	r.tt.Until("dialog", r.tt.Shows("Disconnect Last.fm?"))
	// The dialog's confirming button is the last "Disconnect".
	r.confirm()
	r.tt.Until("called", func() bool { return len(f.did()) == 1 })
	if f.did()[0] != "disconnect:lastfm" {
		t.Fatalf("calls %v", f.did())
	}
}

func TestDisconnectQuestionStaysUpBusyThenClosesOrAsksAgain(t *testing.T) {
	f := connectedFake(2)
	f.discHold = make(chan struct{})
	f.discErr = errors.New("keyring locked")
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("Disconnect"))
	r.click("Disconnect Last.fm")
	r.tt.Until("dialog", r.tt.Shows("Disconnect Last.fm?"))
	r.confirm()
	r.tt.Until("called", func() bool { return len(f.did()) == 1 })
	r.tt.Until("busy", r.tt.Shows("Disconnect Last.fm?", "Disconnecting…"))
	if !kit.ConfirmPending() {
		t.Fatal("the question closed while it runs")
	}
	f.discHold <- struct{}{} // it fails
	r.tt.Until("asked again", func() bool { return !r.tt.HasText("Disconnecting…") && r.tt.HasText("Disconnect Last.fm?") })
	f.mu.Lock()
	f.discErr = nil
	f.mu.Unlock()
	r.confirm()
	r.tt.Until("busy again", r.tt.Shows("Disconnecting…"))
	f.discHold <- struct{}{}
	r.tt.Until("closed", func() bool { return !r.tt.HasText("Disconnect Last.fm?") })
	if kit.ConfirmPending() {
		t.Error("the question is still pending")
	}
}

// confirm answers the pending question yes.
func (r *connRig) confirm() {
	r.t.Helper()
	if !kit.ConfirmPending() {
		r.t.Fatal("no question pending")
	}
	r.tt.Key(0, ui.KeyEnter) // the confirming button has the focus
	r.tt.Drain()
}

func TestDisconnectWithoutPendingGoesAtOnce(t *testing.T) {
	f := connectedFake(0)
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("page", r.tt.Shows("Disconnect"))
	r.click("Disconnect Last.fm")
	r.tt.Until("called", func() bool { return len(f.did()) == 1 })
	if kit.ConfirmPending() {
		t.Fatal("asked with nothing pending")
	}
}

// The note about unencrypted scrobbler sign-ins names this OS's keyring.
func TestUnencryptedNoteIsWordedForTheOS(t *testing.T) {
	for goos, want := range map[string]string{
		"linux":   "Start GNOME Keyring or KWallet and Sona moves them into the keyring.",
		"darwin":  "Sona couldn't use your Keychain, so it keeps them in files only your user can read.",
		"windows": "Sona couldn't use Windows Credential Manager, so it keeps them in files only your user can read.",
	} {
		t.Run(goos, func(t *testing.T) {
			f := &connFake{status: services.ScrobbleStatus{
				LastFM:       services.ScrobblerStatus{Available: true, Connected: true},
				ListenBrainz: services.ScrobblerStatus{Available: true},
				Unencrypted:  true,
			}}
			r := newConnRig(t, f, services.IntegrationsStatus{}, services.Capabilities{OS: goos})
			r.tt.Until("the note", r.tt.Shows("Your scrobbler sign-ins are stored unencrypted", want))
			if goos != "linux" && (r.tt.HasText("GNOME") || r.tt.HasText("Secret Service")) {
				t.Errorf("a Linux keyring on %s: %q", goos, r.tt.Texts())
			}
		})
	}
}

func TestReconnectStateAndUnencryptedNote(t *testing.T) {
	f := &connFake{status: services.ScrobbleStatus{
		LastFM:       services.ScrobblerStatus{Available: true, Connected: true, NeedsReconnect: true},
		ListenBrainz: services.ScrobblerStatus{Available: true},
		Unencrypted:  true,
	}}
	r := newConnRig(t, f, services.IntegrationsStatus{}, allCaps)
	r.tt.Until("state", r.tt.Shows("Last.fm no longer accepts Sona's sign-in. Reconnect to keep scrobbling.", "Reconnect",
		"Your scrobbler sign-ins are stored unencrypted"))
}

func TestPrivateListeningLineAndScrobbleStatusError(t *testing.T) {
	r := newConnRig(t, &connFake{}, services.IntegrationsStatus{}, allCaps)
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "setting"})
	r.tt.Until("line", r.tt.Shows("Nothing is scrobbled while private listening is on."))
}

func discordRig(t *testing.T, d services.DiscordStatus, caps services.Capabilities) *connRig {
	return newConnRig(t, &connFake{}, services.IntegrationsStatus{Discord: d}, caps)
}

func TestDiscordNotConfiguredDisablesTheSwitch(t *testing.T) {
	r := discordRig(t, services.DiscordStatus{}, allCaps)
	r.tt.Until("row", r.tt.Shows("Discord status", "Not available in this build: it was made without a Discord application ID."))
	r.toggle("discord-presence")
	r.tt.Drain()
	if r.saved().Integrations.DiscordPresence {
		t.Fatal("a disabled switch saved")
	}
}

func TestDiscordSwitchSavesAndDescribesTheStates(t *testing.T) {
	base := "Shows the song you're playing, with its artwork and time, as your Discord activity. "
	r := discordRig(t, services.DiscordStatus{Configured: true, Connected: true}, allCaps)
	r.tt.Until("off", r.tt.Shows(base+"Cleared when you pause."))
	r.toggle("discord-presence")
	r.tt.Until("saved", func() bool { return r.saved().Integrations.DiscordPresence })
	r.tt.Until("connected", r.tt.Shows(base+"Cleared when you pause. Connected to Discord."))
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "setting"})
	r.tt.Until("private", r.tt.Shows(base+"Nothing is shown while private listening is on."))
	r.toggle("discord-presence")
	r.tt.Until("saved off", func() bool { return !r.saved().Integrations.DiscordPresence })
}

func TestDiscordUnreachable(t *testing.T) {
	r := discordRig(t, services.DiscordStatus{Configured: true}, allCaps)
	r.setSaved(func(s *config.Settings) { s.Integrations.DiscordPresence = true })
	r.tt.Until("state", r.tt.Shows("Discord isn't reachable yet; Sona keeps trying while music plays."))
}

func TestDiscordUnavailableOnThisPlatform(t *testing.T) {
	r := discordRig(t, services.DiscordStatus{}, services.Capabilities{OS: "linux"})
	r.tt.Until("row", r.tt.Shows("Discord status", "Not available on Linux yet."))
}

// toggle puts the focus on a row's control (as Find does) and presses Space.
func (r *connRig) toggle(id string) {
	r.t.Helper()
	r.s.focusSetting(id)
	r.tt.Drain()
	r.tt.Key(0, ui.KeySpace)
	r.tt.Drain()
}
