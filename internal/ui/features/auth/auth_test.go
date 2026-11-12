package auth

import (
	"context"
	"image"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeAPI is the AuthService the views talk to: it answers with a status the
// test sets and counts the calls a menu or the dialog makes. The calls a test
// never reaches panic on the nil interface.
type fakeAPI struct {
	account.API

	mu     sync.Mutex
	status services.AuthStatus
	calls  map[string]int
	// revalidateErr is what Revalidate fails with.
	revalidateErr error
}

func (a *fakeAPI) Status() services.AuthStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.status
}

func (a *fakeAPI) set(s services.AuthStatus) {
	a.mu.Lock()
	a.status = s
	a.mu.Unlock()
}

func (a *fakeAPI) call(name string) {
	a.mu.Lock()
	a.calls[name]++
	a.mu.Unlock()
}

func (a *fakeAPI) count(name string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls[name]
}

func (a *fakeAPI) Revalidate(context.Context) error { a.call("Revalidate"); return a.revalidateErr }
func (a *fakeAPI) RetryKeyring()                    { a.call("RetryKeyring") }
func (a *fakeAPI) SignOut(context.Context) error    { a.call("SignOut"); return nil }
func (a *fakeAPI) CancelSignIn()                    { a.call("CancelSignIn") }

var (
	signedOut = services.AuthStatus{State: "signed_out"}
	signedIn  = services.AuthStatus{
		State: "signed_in", StorefrontID: "gb", StorefrontName: "United Kingdom", Language: "en-GB", Backend: "secret-service",
	}
)

// rig is the account slot of the top bar over a real Env, with the AuthService
// faked.
type rig struct {
	t   *testing.T
	h   *uitest.Harness
	api *fakeAPI
	f   *feature
	mod module.Module
	tt  *uitest.Session
}

func newRig(t *testing.T, st services.AuthStatus, opts ...uitest.Option) *rig {
	t.Helper()
	h := uitest.New(t, opts...)
	// The harness's real auth manager restores asynchronously and emits
	// auth:changed as it goes: let that finish and be delivered (to nobody)
	// before the feature subscribes, so only the test's events reach it.
	real := services.Service[services.AuthService](h.Core)
	delivered := false
	off := store.On(h.Bus, services.EventAuthChanged, func(st services.AuthStatus) { delivered = delivered || st.State == "signed_out" })
	for deadline := time.Now().Add(5 * time.Second); !delivered; time.Sleep(5 * time.Millisecond) {
		h.Main.Drain()
		if time.Now().After(deadline) {
			t.Fatalf("the harness's auth manager never settled (%s)", real.Status().State)
		}
	}
	off()
	kit.SetToastPoster(h.Main.Post)
	t.Cleanup(func() { kit.SetToastPoster(nil); kit.CancelConfirm() })
	api := &fakeAPI{status: st, calls: map[string]int{}}
	d := account.NewDeps(h.Env)
	d.API = api
	r := &rig{t: t, h: h, api: api}
	r.f = newFeature(h.Env, d)
	r.mod = r.f.module()
	store.Set(h.Env.Queries, store.KeyAuth, st)
	store.Set(h.Env.Queries, store.KeyPrivate, services.PrivateState{Source: "setting"})
	r.mod.Init()
	r.tt = h.Tester(t, r.view, 900, 700)
	r.tt.Drain()
	return r
}

// view is a top bar with the account slot at its trailing end and the
// shell's overlay layer (the sign-out confirm, the toasts).
func (r *rig) view(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Height(48).FillWidth().Padding(0, 16).AlignItems(ui.Center).Background(theme.C("surface")).Children(func() {
			ui.Box(c).Grow(1).Height(1)
			r.mod.Slots.TopBarAccount(c)
		})
	})
	kit.OverlayLayer(c)
}

// settle runs the frames a menu's choice needs to take effect.
func (r *rig) settle() {
	r.t.Helper()
	for range 4 {
		r.tt.Advance(60 * time.Millisecond)
		r.tt.Drain()
	}
}

// openMenu opens the menu from the keyboard, as a user does: Tab to the
// avatar, Enter.
func (r *rig) openMenu() {
	r.t.Helper()
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Account") {
		r.t.Fatalf("the avatar did not take the focus; texts %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEnter)
	r.settle()
	if !r.f.menu.IsOpen() {
		r.t.Fatal("Enter on the avatar did not open the menu")
	}
}

// click clicks the element labeled s and lets what it did take effect.
func (r *rig) click(s string) {
	r.t.Helper()
	if err := r.tt.Click(s); err != nil {
		r.t.Fatal(err)
	}
	r.settle()
}

func (r *rig) shows(texts ...string) {
	r.t.Helper()
	for _, s := range texts {
		if !r.tt.HasText(s) {
			r.t.Errorf("%q is not shown; texts %q", s, r.tt.Texts())
		}
	}
}

func (r *rig) hides(texts ...string) {
	r.t.Helper()
	for _, s := range texts {
		if _, ok := r.tt.Find(s); ok {
			r.t.Errorf("%q is shown", s)
		}
	}
}

func TestModuleDeclaresItsShortcutAndSlot(t *testing.T) {
	r := newRig(t, signedOut)
	if r.mod.ID != "auth" {
		t.Errorf("ID %q", r.mod.ID)
	}
	if r.mod.Slots.TopBarAccount == nil {
		t.Error("no account slot")
	}
	if len(r.mod.Shortcuts) != 1 {
		t.Fatalf("%d shortcuts", len(r.mod.Shortcuts))
	}
	s := r.mod.Shortcuts[0]
	if s.ID != "auth.account" || s.Group != "general" || len(s.Keys) != 0 || s.Icon != "circle-user" {
		t.Errorf("shortcut %+v", s)
	}
	if s.Label() != "Account settings" || s.Keywords() != "account apple music sign in sign out storefront keyring" {
		t.Errorf("shortcut words %q, %q", s.Label(), s.Keywords())
	}
	s.Run()
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Error("the shortcut did not open the account dialog")
	}
}

func TestStateLine(t *testing.T) {
	st := func(f func(*services.AuthStatus)) services.AuthStatus {
		s := signedIn
		f(&s)
		return s
	}
	for _, c := range []struct {
		name string
		s    services.AuthStatus
		want string
	}{
		{"keyring waiting", st(func(s *services.AuthStatus) { s.Keyring = "waiting" }), "Saving your sign-in in your keyring; unlock it if it asks."},
		{"validating", st(func(s *services.AuthStatus) { s.State = "validating" }), "Checking your sign-in…"},
		{"storefront name", signedIn, "Signed in · United Kingdom"},
		{"storefront id", st(func(s *services.AuthStatus) { s.StorefrontName = "" }), "Signed in · GB"},
		{"no storefront", st(func(s *services.AuthStatus) { s.StorefrontName, s.StorefrontID = "", "" }), "Signed in to Apple Music"},
		{"restoring", services.AuthStatus{State: "restoring"}, "Signing in…"},
		{"signing in", services.AuthStatus{State: "signing_in"}, "Signing in…"},
		{"needs reauth", services.AuthStatus{State: "needs_reauth"}, "Apple stopped accepting Sona's sign-in"},
		{"error", services.AuthStatus{State: "error"}, "Apple Music isn't accepting Sona's requests"},
		{"keyring unavailable", services.AuthStatus{State: "signed_out", Keyring: "unavailable"}, "Sona couldn't read your sign-in from your keyring"},
		{"signed out", signedOut, "Not signed in"},
	} {
		if got := stateLine(c.s); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSignedOutMenuOffersSignInAndOpensTheDialog(t *testing.T) {
	r := newRig(t, signedOut)
	r.openMenu()
	r.shows("Apple Music", "Not signed in")
	r.hides("Sign out", "Account settings", "Check now", "Try again")
	if _, ok := r.tt.Find("Sign in"); !ok {
		t.Fatalf("no Sign in row; texts %q", r.tt.Texts())
	}
	r.click("Sign in")
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Fatal("Sign in did not open the account dialog")
	}
	r.shows("Sign in to Apple Music")
	if !r.tt.Focused("Apple Account") {
		t.Errorf("the Apple Account field has no focus; texts %q", r.tt.Texts())
	}
}

func TestNothingIsOfferedWhileRestoring(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "restoring"})
	r.openMenu()
	r.shows("Signing in…")
	r.hides("Sign in", "Account settings", "Sign out")
}

func TestSignedInMenuShowsTheAccountAndItsNotes(t *testing.T) {
	st := signedIn
	st.Restricted, st.Offline, st.Unencrypted = true, true, true
	r := newRig(t, st)
	r.openMenu()
	r.shows(
		"Signed in · United Kingdom",
		"Offline. Sona keeps using your saved sign-in.",
		"Explicit content is restricted on this account; Sona hides it.",
		"Your token is stored unencrypted",
	)
	r.hides("Sign in", "Try again")
	r.click("Check now")
	r.tt.Until("the check", func() bool { return r.api.count("Revalidate") == 1 })
}

func TestSignedOutUnencryptedTokenNote(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", Unencrypted: true})
	r.openMenu()
	r.shows("Signing in now stores your token unencrypted")
	r.hides("Your token is stored unencrypted", "Check now")
	if _, ok := r.tt.Find("Try again"); !ok {
		t.Error("no Try again row for the unencrypted fallback")
	}
}

// The header is a 20 high line of medium ink (32 with its padding) and the
// facts under it lead with a glyph: 14 and a 6 gap before the text, which the
// Private listening note under its switch starts without.
func TestMenuHeaderAndNotesAreStyled(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", Unencrypted: true})
	r.openMenu()
	head, ok := r.tt.Find("Apple Music")
	if !ok || head.H != 32 {
		t.Errorf("the header is %v (found %v), want a 20 high line with 6 above and below", head, ok)
	}
	plain, ok := r.tt.Find("Plays stay out of your listening history, scrobbles and Discord.")
	if !ok || plain.X != head.X+8 {
		t.Errorf("a note without an icon is at %v (found %v), want the header's text start %v", plain.X, ok, head.X+8)
	}
	if state, ok := r.tt.Find("Not signed in"); !ok || state.X != plain.X+14+6 {
		t.Errorf("the state line starts at %v (found %v), want it after its glyph (%v)", state.X, ok, plain.X+14+6)
	}
	note, ok := r.tt.Find("Signing in now stores your token unencrypted")
	if !ok || note.X != plain.X+14+6 {
		t.Errorf("the unencrypted note starts at %v (found %v), want it after the icon (%v)", note.X, ok, plain.X+14+6)
	}
}

func TestCheckNowFailureToasts(t *testing.T) {
	st := signedIn
	st.Offline = true
	r := newRig(t, st)
	r.api.revalidateErr = services.ErrNotReleased
	r.openMenu()
	r.click("Check now")
	r.tt.Until("the failure's toast", r.tt.Shows("Couldn't reach Apple", i18n.ErrorText(services.CodeNotReleased)))
}

func TestSignedInMenuOpensTheAccountSettings(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.hides("Sign in")
	r.click("Account settings")
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Fatal("Account settings did not open the dialog")
	}
	r.shows("Signed in to Apple Music", "British English")
}

func TestSignOutAsksBeforeSigningOut(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.click("Sign out")
	r.shows("Sign out of Apple Music?")
	if n := r.api.count("SignOut"); n != 0 {
		t.Fatalf("signed out %d times before the confirm", n)
	}
	r.click("Sign out") // the confirm's button; the menu is gone
	r.tt.Until("SignOut", func() bool { return r.api.count("SignOut") == 1 })
	r.tt.Until("the confirm to close", func() bool { return !r.tt.HasText("Sign out of Apple Music?") })
}

func TestSignOutCancelDoesNotSignOut(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.click("Sign out")
	r.click("Cancel")
	if r.tt.HasText("Sign out of Apple Music?") {
		t.Error("Cancel left the confirm open")
	}
	if n := r.api.count("SignOut"); n != 0 {
		t.Errorf("signed out %d times", n)
	}
}

func TestKeyringTroubleOffersTryAgain(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", Keyring: "unavailable"})
	r.openMenu()
	r.shows("Sona couldn't read your sign-in from your keyring")
	r.click("Try again")
	r.tt.Until("RetryKeyring", func() bool { return r.api.count("RetryKeyring") == 1 })
}

func TestPrivateListeningSwitchesAndStaysOpen(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.shows("Plays stay out of your listening history, scrobbles and Discord.")
	r.click("Private listening")
	// The menu never closed: it is open right after the choice, not
	// reopened once Go has answered.
	if !r.f.menu.IsOpen() {
		t.Fatal("choosing Private listening closed the menu")
	}
	r.tt.Until("private listening on", func() bool {
		p, _ := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate)
		return p.On
	})
	if !r.f.menu.IsOpen() {
		t.Fatal("the menu closed when Go answered")
	}
	// Still on the same row, still choosable: a second click switches it off.
	r.click("Private listening")
	r.tt.Until("private listening off", func() bool {
		p, _ := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate)
		return !p.On
	})
	if !r.f.menu.IsOpen() {
		t.Error("the menu closed after the second choice")
	}
}

func TestDevelopmentBuildsPrivateNote(t *testing.T) {
	r := newRig(t, signedIn)
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "development"})
	r.openMenu()
	r.shows("Private listening is on for this development build. Turning it off lasts until Sona quits.")
}

func TestForcedPrivateListeningIgnoresClicksAndSaysWhy(t *testing.T) {
	r := newRig(t, signedIn)
	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "environment"})
	r.openMenu()
	r.shows("On because Sona was started with SONA_PRIVATE=1")
	r.hides("Plays stay out of your listening history, scrobbles and Discord.")
	if err := r.tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	// A call would turn it off (the service here is not forced): give it
	// the time it would take.
	for range 5 {
		time.Sleep(10 * time.Millisecond)
		r.tt.Drain()
	}
	if p, _ := store.Peek[services.PrivateState](r.h.Env.Queries, store.KeyPrivate); !p.On || p.Source != "environment" {
		t.Errorf("a click changed the forced state: %+v", p)
	}
	if !r.f.menu.IsOpen() {
		t.Error("a click on the forced row closed the menu")
	}
}

func TestPrivateListeningIsOffUntilTheStateIsKnown(t *testing.T) {
	r := newRig(t, signedIn)
	// No data and nothing to ask: Go's answer never comes.
	r.h.Env.Queries.Reset()
	store.Set(r.h.Env.Queries, store.KeyAuth, signedIn)
	r.h.Env.Core = nil
	r.openMenu()
	if err := r.tt.Click("Private listening"); err != nil {
		t.Fatal(err)
	}
	r.settle()
	if !r.f.menu.IsOpen() {
		t.Error("the unavailable row closed the menu")
	}
}

func TestEscapeClosesTheMenuAndReturnsFocus(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	if r.f.menu.IsOpen() {
		t.Fatal("Escape left the menu open")
	}
	if !r.tt.Focused("Account") {
		t.Error("the focus did not return to the avatar")
	}
}

// dotAround counts the red pixels around the avatar's top end: the
// attention dot, in --danger.
func dotAround(img *image.RGBA, r ui.Rect) int {
	n := 0
	for y := int(r.Y) - 6; y < int(r.Y)+int(r.H)/2; y++ {
		for x := int(r.X) + int(r.W)/2; x < int(r.X)+int(r.W)+6; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			p := img.RGBAAt(x, y)
			if p.R > 150 && p.G < 90 && p.B < 90 {
				n++
			}
		}
	}
	return n
}

func TestTheAvatarsDotMeansTheAccountNeedsAttention(t *testing.T) {
	for _, c := range []struct {
		name string
		s    services.AuthStatus
		want bool
	}{
		{"signed in", signedIn, false},
		{"signed out", signedOut, false},
		{"needs reauth", services.AuthStatus{State: "needs_reauth"}, true},
		{"error", services.AuthStatus{State: "error"}, true},
		{"keyring unavailable", services.AuthStatus{State: "signed_out", Keyring: "unavailable"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, c.s)
			rect, ok := r.tt.Find("Account")
			if !ok {
				t.Fatal("no avatar")
			}
			if got := dotAround(r.tt.Image(), rect) > 0; got != c.want {
				t.Errorf("dot shown %v, want %v", got, c.want)
			}
		})
	}
}

func TestAvatarTooltipSaysWhereTheAccountStands(t *testing.T) {
	r := newRig(t, signedIn)
	// Hovering shows the tooltip: "Account" and the state line.
	rect, ok := r.tt.Find("Account")
	if !ok {
		t.Fatal("no avatar")
	}
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Advance(time.Second)
	r.tt.Drain()
	r.shows("Signed in · United Kingdom")
}

func TestDialogFollowsSurfaces(t *testing.T) {
	r := newRig(t, signedOut)
	if r.tt.HasText("Sign in to Apple Music") {
		t.Fatal("the dialog is open before anyone asked")
	}
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	r.shows("Sign in to Apple Music")
	if !r.tt.Focused("Apple Account") {
		t.Errorf("the Apple Account field has no focus; texts %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	if r.h.Env.Surfaces.AccountOpen() {
		t.Error("Escape did not close the account dialog")
	}
	if r.tt.HasText("Sign in to Apple Music") {
		t.Error("the dialog is still shown")
	}

	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	r.click("Close")
	if r.h.Env.Surfaces.AccountOpen() {
		t.Error("the X did not close the account dialog")
	}

	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	r.tt.ClickAt(5, 5)
	r.settle()
	if r.h.Env.Surfaces.AccountOpen() {
		t.Error("a click on the scrim did not close the account dialog")
	}
}

// The body scrolls inside a short window, as max-h-[calc(100%-2rem)]
// overflow-y-auto does: the panel stays inside it, the X and the Apple Account
// field in sight, and the end of the form below, reached by scrolling.
func TestDialogBodyScrollsInAShortWindow(t *testing.T) {
	r := newRig(t, signedOut)
	r.tt.SetSize(900, 300)
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	x, ok := r.tt.Find("Close")
	if !ok || x.Y < 0 {
		t.Fatalf("the dialog's X is at %v (found %v): the panel is taller than the window", x, ok)
	}
	if f, ok := r.tt.Find("Apple Account"); !ok || f.Y+f.H > 300 {
		t.Errorf("the Apple Account field is at %v (found %v), not in sight", f, ok)
	}
	if b, ok := r.tt.Find("Sign in"); ok && b.Y+b.H <= 300 {
		t.Errorf("the whole form fits a 300 high window: nothing is capped (Sign in at %v)", b)
	}
}

// Closing the dialog drops what was typed in it, as React unmounted it: the
// form is empty when it opens again.
func TestClosingTheDialogForgetsWhatWasTyped(t *testing.T) {
	r := newRig(t, signedOut)
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	r.tt.Type("someone@example.com")
	r.settle()
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	if r.tt.HasText("someone@example.com") {
		t.Error("the account name typed before closing is still in the form")
	}
}

func TestDialogShowsTheReauthNotice(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "needs_reauth"})
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	r.shows("Apple stopped accepting Sona's sign-in", "Sign in to Apple Music")
}

func TestDialogStaysOpenAfterSigningIn(t *testing.T) {
	r := newRig(t, signedOut)
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	r.api.set(signedIn)
	r.h.Env.Bus.Emit(services.EventAuthChanged, signedIn)
	r.tt.Until("the signed-in view", r.tt.Shows("Signed in to Apple Music"))
	if !r.h.Env.Surfaces.AccountOpen() {
		t.Error("the dialog closed after the sign-in")
	}
}

func TestDialogWhileSetupIsOpenGoesToTheSetupStep(t *testing.T) {
	r := newRig(t, signedOut)
	r.h.Env.Surfaces.OpenSetup(store.SetupWelcome)
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	if r.h.Env.Surfaces.AccountOpen() || r.h.Env.Surfaces.SetupStep() != store.SetupAccount {
		t.Error("the account did not become the setup's step")
	}
	if r.tt.HasText("Sign in to Apple Music") {
		t.Error("the dialog opened over the setup")
	}
}

func TestAuthChangedWritesTheStatusAndInvalidates(t *testing.T) {
	r := newRig(t, signedOut)
	var cached []store.Key
	for _, k := range cachedKeys {
		if keyPath(k) != "auth" { // the auth query holds the status itself, not an int
			cached = append(cached, k)
		}
	}
	for _, k := range cached {
		store.Set(r.h.Env.Queries, k, 1)
	}

	var log fetchLog
	pump := func() { r.h.Main.Drain() }
	// An intermediate state changes nothing.
	r.h.Env.Bus.Emit(services.EventAuthChanged, services.AuthStatus{State: "signing_in"})
	r.tt.Drain()
	if got := staleKeys(t, r.h.Env.Queries, cached, &log, pump); len(got) != 0 {
		t.Errorf("signing_in made %v stale", got)
	}
	if st, _ := store.Peek[services.AuthStatus](r.h.Env.Queries, store.KeyAuth); st.State != "signing_in" {
		t.Errorf("the status is %q", st.State)
	}

	r.h.Env.Bus.Emit(services.EventAuthChanged, signedIn)
	r.tt.Drain()
	if st, _ := store.Peek[services.AuthStatus](r.h.Env.Queries, store.KeyAuth); st.State != "signed_in" {
		t.Errorf("the status is %q after signing in", st.State)
	}
	got := staleKeys(t, r.h.Env.Queries, cached, &log, pump)
	for _, want := range []string{"library/songs/title/false/0", "catalog/lyrics/1097862703", "diagnostics"} {
		if !slices.Contains(got, want) {
			t.Errorf("%s is not stale after signing in: %v", want, got)
		}
	}
	if slices.Contains(got, "settings") {
		t.Errorf("settings are stale: %v", got)
	}
}

func TestInvalidationsCompareWithThePreviousEventNotTheCache(t *testing.T) {
	r := newRig(t, signedOut)
	// Another feature's handler of the same event writes the status first.
	store.On(r.h.Env.Bus, services.EventAuthChanged, func(s services.AuthStatus) {
		store.Set(r.h.Env.Queries, store.KeyAuth, s)
	})
	store.Set(r.h.Env.Queries, store.Key{"library", "albums"}, 1)
	r.h.Env.Bus.Emit(services.EventAuthChanged, signedIn)
	r.tt.Drain()
	if got := staleKeys(t, r.h.Env.Queries, []store.Key{{"library", "albums"}}, &fetchLog{}, func() { r.h.Main.Drain() }); len(got) != 1 {
		t.Errorf("signing in left the library fresh: %v", got)
	}
}
