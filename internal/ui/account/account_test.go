package account

import (
	"context"
	"errors"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeAPI is the AuthService the views are tested against: it records what
// it was asked and answers with what the test set.
type fakeAPI struct {
	mu         sync.Mutex
	status     services.AuthStatus
	statusGate chan struct{} // Status waits for it to close
	calls      []string
	onPassword func(account, password string) (*services.SecondFactor, error)
	onSubmit   func(code string, phoneID int, mode string) error
	onPhone    func(phoneID int, mode string) error
	onResend   func() error
	onSignIn   func() error
	onToken    func(token string) error
	onSignOut  func() error
	onRevalid  func() error
	onTouchID  func(on bool) error
	link       string
	linkErr    error
}

func (f *fakeAPI) rec(format string, a ...any) {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
	f.mu.Unlock()
}

// count is how many calls start with prefix.
func (f *fakeAPI) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeAPI) last(prefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range slices.Backward(f.calls) {
		if strings.HasPrefix(c, prefix) {
			return c
		}
	}
	return ""
}

func (f *fakeAPI) Status() services.AuthStatus {
	if f.statusGate != nil {
		<-f.statusGate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeAPI) SignInWithPassword(_ context.Context, account, password string) (*services.SecondFactor, error) {
	f.rec("password %s/%s", account, password)
	if f.onPassword != nil {
		return f.onPassword(account, password)
	}
	return nil, nil
}

func (f *fakeAPI) SubmitSecurityCode(_ context.Context, code string, phoneID int, mode string) error {
	f.rec("submit %s %d %s", code, phoneID, mode)
	if f.onSubmit != nil {
		return f.onSubmit(code, phoneID, mode)
	}
	return nil
}

func (f *fakeAPI) SendCodeToPhone(_ context.Context, phoneID int, mode string) error {
	f.rec("phone %d %s", phoneID, mode)
	if f.onPhone != nil {
		return f.onPhone(phoneID, mode)
	}
	return nil
}

func (f *fakeAPI) ResendSecurityCode(context.Context) error {
	f.rec("resend")
	if f.onResend != nil {
		return f.onResend()
	}
	return nil
}

func (f *fakeAPI) SignIn(context.Context) error {
	f.rec("signin")
	if f.onSignIn != nil {
		return f.onSignIn()
	}
	return nil
}

func (f *fakeAPI) SignInLink() (string, error) {
	f.rec("link")
	return f.link, f.linkErr
}

func (f *fakeAPI) CancelSignIn() { f.rec("cancel") }

func (f *fakeAPI) SignInWithToken(_ context.Context, token string) error {
	f.rec("token %s", token)
	if f.onToken != nil {
		return f.onToken(token)
	}
	return nil
}

func (f *fakeAPI) SignOut(context.Context) error {
	f.rec("signout")
	if f.onSignOut != nil {
		return f.onSignOut()
	}
	return nil
}

func (f *fakeAPI) Revalidate(context.Context) error {
	f.rec("revalidate")
	if f.onRevalid != nil {
		return f.onRevalid()
	}
	return nil
}

func (f *fakeAPI) RetryKeyring() { f.rec("retry") }

func (f *fakeAPI) SetBiometricUnlock(_ context.Context, on bool) error {
	f.rec("touchid %v", on)
	if f.onTouchID != nil {
		return f.onTouchID(on)
	}
	return nil
}

var signedOut = services.AuthStatus{State: "signed_out"}

// rig is a Section on a fake API in a window.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	api    *fakeAPI
	sec    *Section
	tt     *uitest.Session
	hosted bool
}

// rigOpts shape a rig.
type rigOpts struct {
	// status is what the status query holds (and Status answers) from the start.
	status *services.AuthStatus
	// fetch is what Status answers when the query holds nothing yet.
	fetch services.AuthStatus
	// gate makes Status wait for it to close.
	gate chan struct{}
	// noAPI builds the Section without a service.
	noAPI bool
	// os is the GOOS the keyring notices are worded for ("" is Linux's).
	os string
}

// newRig shows a Section whose status query already holds st.
func newRig(t *testing.T, st services.AuthStatus) *rig {
	t.Helper()
	return buildRig(t, rigOpts{status: &st})
}

func buildRig(t *testing.T, o rigOpts) *rig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	fake := &fakeAPI{status: o.fetch, statusGate: o.gate}
	if o.status != nil {
		fake.status = *o.status
		store.Set(h.Env.Queries, store.KeyAuth, *o.status)
	}
	r := &rig{t: t, h: h, api: fake}
	d := Deps{API: fake, Queries: h.Env.Queries, Post: h.Env.Post, OpenURL: h.Env.OpenURL, Copy: h.Env.Copy, Log: h.Env.Log, OS: o.os}
	if o.noAPI {
		d.API = nil
	}
	r.sec = New(d)
	kit.SetToastPoster(h.Main.Post)
	t.Cleanup(func() { kit.CancelConfirm(); kit.CloseToast("") })
	r.tt = h.Tester(t, func(c *ui.Context) {
		ui.Column(c.Key("page")).FillWidth().Padding(24).Children(func() {
			r.sec.Build(c, r.hosted)
			r.sec.FootAction(c)
		})
		kit.ConfirmHost(c)
		kit.Toaster(c)
	}, 720, 1100)
	return r
}

// has fails unless an element shows exactly each label; lacks the reverse.
func (r *rig) has(labels ...string) {
	r.t.Helper()
	for _, s := range labels {
		if _, ok := r.tt.Find(s); !ok {
			r.t.Errorf("no element shows %q; texts %q", s, r.tt.Texts())
		}
	}
}

func (r *rig) lacks(labels ...string) {
	r.t.Helper()
	for _, s := range labels {
		if _, ok := r.tt.Find(s); ok {
			r.t.Errorf("an element shows %q; texts %q", s, r.tt.Texts())
		}
	}
}

// status is the auth:changed handler's write.
func (r *rig) status(st services.AuthStatus) {
	r.t.Helper()
	r.api.mu.Lock()
	r.api.status = st
	r.api.mu.Unlock()
	store.Set(r.h.Env.Queries, store.KeyAuth, st)
	r.tt.Drain()
}

func (r *rig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatalf("%v; texts %q", err, r.tt.Texts())
	}
	r.tt.Drain()
}

func (r *rig) typeInto(label, s string) {
	r.t.Helper()
	r.click(label)
	r.tt.Type(s)
	r.tt.Drain()
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
		if r.tt.HasText(s) {
			r.t.Errorf("%q is shown; texts %q", s, r.tt.Texts())
		}
	}
}

func (r *rig) waitCalls(prefix string, n int) {
	r.t.Helper()
	r.tt.Until(fmt.Sprintf("%d %q calls", n, prefix), func() bool { return r.api.count(prefix) >= n })
}

// signIn fills the credentials form and presses Sign in.
func (r *rig) signIn(account, password string) {
	r.t.Helper()
	r.typeInto("Apple Account", account)
	r.typeInto("Password", password)
	r.click("Sign in")
	r.waitCalls("password", 1)
}

func TestSignedOutShowsTheCredentialsForm(t *testing.T) {
	r := newRig(t, signedOut)
	r.has(
		"Sign in to Apple Music", "Use the Apple Account with your Apple Music subscription.",
		"Apple Account", "The email address or phone number you use with Apple.", "Password",
		"Sign in", "Sona talks to Apple directly. Your password is used once and never saved.",
		"Use a passkey, a security key or a browser you're already signed in to:",
		"Sign in with your browser", "Paste a Music User Token",
	)
}

func TestHostedFormLeavesTheTitleAndSubmitToTheHost(t *testing.T) {
	r := newRig(t, signedOut)
	r.hosted = true
	r.tt.Frame()
	r.lacks("Sign in to Apple Music", "Use the Apple Account with your Apple Music subscription.")
	r.has("Apple Account", "Password", "Sign in", "Sona talks to Apple directly. Your password is used once and never saved.")
	r.typeInto("Apple Account", "a@example.com")
	r.typeInto("Password", "pw")
	r.click("Sign in") // the host's foot button submits the form
	r.waitCalls("password", 1)
	// Off the credentials form, the host has no submit to build.
	r.tt.Until("done", r.tt.Shows("Checking your account with Apple…"))
	r.lacks("Sign in")
}

func TestSubmitNeedsBothFieldsAndCallsWithTheTrimmedAccount(t *testing.T) {
	r := newRig(t, signedOut)
	r.click("Sign in") // disabled: nothing happens
	r.typeInto("Apple Account", "  a@example.com  ")
	r.click("Sign in")
	r.typeInto("Password", "secret pw")
	if n := r.api.count("password"); n != 0 {
		t.Fatalf("%d sign-ins before both fields were filled", n)
	}
	r.click("Sign in")
	r.waitCalls("password", 1)
	if got := r.api.last("password"); got != "password a@example.com/secret pw" {
		t.Errorf("call %q", got)
	}
	if r.sec.flow != nil && r.sec.flow.password != "" {
		t.Error("the password was kept in the field")
	}
	// Signed in without a second factor: the form waits for the status.
	r.tt.Until("done", r.tt.Shows("Checking your account with Apple…"))
}

func TestEnterInTheFieldsSubmits(t *testing.T) {
	r := newRig(t, signedOut)
	r.typeInto("Apple Account", "a@example.com")
	r.typeInto("Password", "pw")
	r.tt.Key(0, ui.KeyEnter)
	r.waitCalls("password", 1)
}

func TestWrongPasswordClearsThePasswordAndKeepsTheAccount(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, appleid.ErrInvalidCredentials }
	r.signIn("a@example.com", "wrong")
	r.tt.Until("error", r.tt.Shows("That Apple Account or password isn't right"))
	r.shows("Check the email or phone number and the password, then try again.", "Sign in")
	if f := r.sec.flow; f == nil || f.password != "" || f.account != "a@example.com" {
		t.Errorf("fields after the attempt: %+v", f)
	}
	if got := r.tt.Announcements(); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "That Apple Account or password isn't right") }) {
		t.Errorf("the error was not announced: %q", got)
	}
}

func TestTwoFactorWrongCodeThenRightCode(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return deviceFactor, nil }
	r.api.onSubmit = func(code string, _ int, _ string) error {
		if code == "123456" {
			return appleid.ErrIncorrectCode
		}
		return nil
	}
	r.signIn("a@example.com", "pw")
	r.tt.Until("code step", r.tt.Shows("Enter the verification code"))
	r.shows("Apple sent a 6-digit code to the devices signed in to this Apple Account. Enter it here to finish signing in.", "6 digits")
	r.has("Code", "Verify", "Send a new code", "Cancel", "Apple device", "Text message", "Phone call")
	r.lacks("No Apple device is signed in to this account, so Apple sends codes by text or call.")

	// Not the last digit: nothing is submitted; non-digits are dropped.
	r.typeInto("Code", "12a3-4")
	if r.sec.flow.code != "1234" || r.api.count("submit") != 0 {
		t.Fatalf("code %q, %d submits", r.sec.flow.code, r.api.count("submit"))
	}
	r.tt.Drain()
	// The last digit submits by itself; a wrong code clears the field, keeps
	// the step and shows the error.
	r.tt.Type("56")
	r.waitCalls("submit", 1)
	if got := r.api.last("submit"); got != "submit 123456 0 sms" {
		t.Errorf("submit %q", got)
	}
	r.tt.Until("wrong code", r.tt.Shows("That code isn't right"))
	r.shows("Enter the verification code", "Enter the latest code you received. Older codes stop working once a new one is sent.")
	if r.sec.flow.code != "" || r.sec.flow.step.Kind != StepCode || r.sec.flow.step.Busy {
		t.Errorf("after a wrong code: code %q, step %+v", r.sec.flow.code, r.sec.flow.step)
	}

	// The right one finishes.
	r.typeInto("Code", "654321")
	r.waitCalls("submit", 2)
	if got := r.api.last("submit"); got != "submit 654321 0 sms" {
		t.Errorf("submit %q", got)
	}
	r.tt.Until("done", r.tt.Shows("Checking your account with Apple…"))
}

func TestCodeLengthFollowsTheFactor(t *testing.T) {
	f := *deviceFactor
	f.CodeLength = 4
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: &f})
	r.shows("Apple sent a 4-digit code to the devices signed in to this Apple Account. Enter it here to finish signing in.", "4 digits")
	r.typeInto("Code", "1234")
	r.waitCalls("submit", 1)
}

func TestVerifyIsDisabledUntilTheLastDigit(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	r.typeInto("Code", "123")
	r.click("Verify")
	if n := r.api.count("submit"); n != 0 {
		t.Fatalf("Verify submitted a short code: %d", n)
	}
	r.tt.Drain()
}

func TestNoDeviceAccountChoosesAPhoneAndGetsATextedCode(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return phonesOnly, nil }
	r.signIn("a@example.com", "pw")
	r.tt.Until("phone choice", r.tt.Shows("Which number should Apple text?"))
	r.shows(
		"Choose one of the trusted phone numbers on this Apple Account.", testPhones[0].Number, testPhones[1].Number,
		"Text me", "Cancel", "No Apple device is signed in to this account, so Apple sends codes by text or call.",
	)
	if r.sec.flow.phoneID != 1 {
		t.Errorf("the first number is not preselected: %d", r.sec.flow.phoneID)
	}
	r.click(testPhones[1].Number)
	r.click("Text me")
	r.waitCalls("phone", 1)
	if got := r.api.last("phone"); got != "phone 2 sms" {
		t.Errorf("phone call %q", got)
	}
	r.tt.Until("code step", r.tt.Shows("Enter the verification code"))
	r.shows("Apple sent a 6-digit code to "+testPhones[1].Number+" by text message.", "Apple sent a code by text message.", "Text again", "Use another number")
	r.typeInto("Code", "123456")
	r.waitCalls("submit", 1)
	if got := r.api.last("submit"); got != "submit 123456 2 sms" {
		t.Errorf("submit %q", got)
	}
}

func TestPhoneCallRouteWithoutDevices(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: phonesOnly})
	r.shows("Which number should Apple text?")
	// The device route is not there for this account; the switcher stays put.
	r.click("Apple device")
	r.shows("Which number should Apple text?")
	r.click("Phone call")
	r.shows("Which number should Apple call?", "Call me")
	r.click("Call me")
	r.waitCalls("phone", 1)
	if got := r.api.last("phone"); got != "phone 1 voice" {
		t.Errorf("phone call %q", got)
	}
	r.tt.Until("code step", r.tt.Shows("Apple is calling +44 ••••• ••••12 with a 6-digit code."))
	r.shows("Apple is calling with your code.", "Call again")
	r.typeInto("Code", "123456")
	r.waitCalls("submit", 1)
	if got := r.api.last("submit"); got != "submit 123456 1 voice" {
		t.Errorf("submit %q", got)
	}
}

func TestPhoneChoiceSendFailureKeepsTheChoice(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: phonesOnly})
	r.api.onPhone = func(int, string) error { return &net.DNSError{Err: "no such host"} }
	r.click("Text me")
	r.tt.Until("network error", r.tt.Shows("Couldn't reach Apple"))
	r.shows("Which number should Apple text?", "Check your connection and try again.")
}

func TestMethodSwitcherOfADeviceAccount(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	// Several numbers: choose which one.
	r.click("Text message")
	r.shows("Which number should Apple text?")
	// And back to the code already on the devices.
	r.click("Apple device")
	r.shows("Enter the verification code")
	if r.sec.flow.step.Target.PhoneID != 0 {
		t.Errorf("target %+v", r.sec.flow.step.Target)
	}
}

func TestMethodSwitcherWithOneNumberSendsAtOnce(t *testing.T) {
	one := &services.SecondFactor{CodeLength: 6, TrustedDevices: true, Phones: testPhones[:1]}
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: one})
	r.click("Phone call")
	r.waitCalls("phone", 1)
	if got := r.api.last("phone"); got != "phone 1 voice" {
		t.Errorf("phone call %q", got)
	}
	r.tt.Until("calling", r.tt.Shows("Apple is calling with your code."))
}

func TestResendFollowsTheMethod(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	r.click("Send a new code")
	r.waitCalls("resend", 1)
	r.tt.Until("notice", r.tt.Shows("A new code is on its way to your devices."))

	p := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: smsOnly})
	p.shows("Apple is calling "+testPhones[1].Number+" with a 6-digit code.", "Call again", "Use another number")
	p.click("Call again")
	p.waitCalls("phone", 1)
	if got := p.api.last("phone"); got != "phone 2 voice" {
		t.Errorf("phone call %q", got)
	}
	p.tt.Until("notice", p.tt.Shows("A new code is on its way."))
	p.click("Use another number")
	p.shows("Which number should Apple call?")
}

func TestResendFailureShowsTheError(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	r.api.onResend = func() error { return &net.DNSError{Err: "no such host"} }
	r.click("Send a new code")
	r.tt.Until("error", r.tt.Shows("Couldn't reach Apple"))
	r.shows("Enter the verification code")
}

func TestCancelInTheCodeStepReturnsToAnEmptyForm(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return deviceFactor, nil }
	r.signIn("a@example.com", "pw")
	r.tt.Until("code step", r.tt.Shows("Enter the verification code"))
	r.click("Cancel")
	r.waitCalls("cancel", 1)
	r.shows("Sign in to Apple Music")
	if f := r.sec.flow; f.account != "" || f.password != "" {
		t.Errorf("the form is not empty: %+v", f)
	}
}

func TestPendingChallengeIsRestoredAndDroppedByStatus(t *testing.T) {
	r := newRig(t, signedOut)
	r.shows("Sign in to Apple Music")
	r.status(services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	r.shows("Enter the verification code")
	// Go dropped the attempt: back to the form with the reason.
	r.status(services.AuthStatus{State: "signed_out", LastErrorCode: services.CodeTimeout})
	r.shows("Sign-in timed out", "No code was entered in time, so Apple closed the attempt. Start again.", "Apple Account")
	r.hides("Enter the verification code")
}

func TestBrowserSignInButtonStartsItAndShowsFailures(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onSignIn = func() error { return mut.ErrCanceled }
	r.click("Sign in with your browser")
	r.waitCalls("signin", 1)
	r.tt.Drain()
	r.hides("This sign-in was cancelled", "Sign-in timed out")

	r.api.onSignIn = func() error { return mut.ErrTimeout }
	r.click("Sign in with your browser")
	r.tt.Until("timeout", r.tt.Shows("Sign-in timed out"))
	r.shows("No code was entered in time, so Apple closed the attempt. Start again.")
}

func TestBrowserWaitingOffersTheLinkAndCancel(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", BrowserSignIn: true})
	r.shows(
		"Finish signing in in your browser",
		"Apple's sign-in page is open in your browser. Sona continues by itself when you're done.",
		"Open the page again", "Copy link", "Cancel",
	)
	r.hides("Sign in with your browser", "Paste a Music User Token", "Apple Account")

	r.api.link = "http://127.0.0.1:9/auth/n/start?t=one"
	r.click("Open the page again")
	r.tt.Until("opened", func() bool { return len(r.h.Opened()) == 1 })
	if got := r.h.Opened(); got[0] != r.api.link {
		t.Errorf("opened %q", got)
	}
	r.click("Copy link")
	r.tt.Until("copied", func() bool { return len(r.h.Copied()) == 1 })
	r.tt.Until("toast", r.tt.Shows("Sign-in link copied"))
	if got := r.h.Copied(); got[0] != r.api.link {
		t.Errorf("copied %q", got)
	}

	// A link that can't be had says so and opens nothing.
	r.api.linkErr = auth.ErrNoBrowserSignIn
	r.click("Open the page again")
	r.tt.Until("toast", r.tt.Shows("Couldn't get a sign-in link"))
	if n := len(r.h.Opened()); n != 1 {
		t.Errorf("%d pages opened", n)
	}

	r.click("Cancel")
	r.waitCalls("cancel", 1)
}

func TestBrowserWaitingWithoutALinkOffersOnlyCancel(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in"})
	r.shows("Finish signing in in your browser", "Cancel")
	r.hides("Open the page again", "Copy link")
}

func TestPasswordExchangeInFlightIsNotABrowserWait(t *testing.T) {
	gate := make(chan struct{})
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { <-gate; return nil, nil }
	r.typeInto("Apple Account", "a@example.com")
	r.typeInto("Password", "pw")
	r.click("Sign in")
	r.waitCalls("password", 1)
	// Go reports signing_in while the exchange runs.
	r.status(services.AuthStatus{State: "signing_in"})
	r.hides("Finish signing in in your browser")
	r.shows("Signing in…")
	close(gate)
	r.tt.Until("done", r.tt.Shows("Checking your account with Apple…"))
}

func TestTokenPasteValidAndRejected(t *testing.T) {
	r := newRig(t, signedOut)
	r.lacks("Music User Token")
	r.click("Paste a Music User Token")
	r.has("Music User Token", "The token from a signed-in Apple Music web session. Sona checks it with Apple and keeps it in your keyring.", "Use this token", "Cancel")

	// Too short: the button is disabled.
	r.typeInto("Music User Token", "short")
	r.click("Use this token")
	if n := r.api.count("token"); n != 0 {
		t.Fatalf("%d token calls for a short token", n)
	}

	tok := strings.Repeat("t", 24)
	r.sec.flow.token = "  " + tok + "  "
	r.tt.Drain()
	r.api.onToken = func(string) error { return fmt.Errorf("auth: x: %w", auth.ErrMUTRejected) }
	r.click("Use this token")
	r.waitCalls("token", 1)
	if got := r.api.last("token"); got != "token "+tok {
		t.Errorf("the token was not trimmed: %q", got)
	}
	r.tt.Until("rejected", r.tt.Shows("Apple didn't accept that token. Copy a fresh one from a signed-in Apple Music web session."))

	r.api.onToken = func(string) error { return &net.DNSError{Err: "no such host"} }
	r.click("Use this token")
	r.tt.Until("network", r.tt.Shows("Couldn't reach Apple to check the token. Check your connection and try again."))

	r.api.onToken = func(string) error { return errors.New("boom") }
	r.click("Use this token")
	r.tt.Until("other", r.tt.Shows("Something went wrong."))
	r.hides("boom")

	// Accepted: the token is cleared.
	r.api.onToken = nil
	r.click("Use this token")
	r.waitCalls("token", 4)
	r.tt.Until("cleared", func() bool { return r.sec.flow == nil || r.sec.flow.token == "" })

	r.click("Cancel")
	r.shows("Paste a Music User Token")
}

func TestTokenFormGoesAwayWithTheCredentialsForm(t *testing.T) {
	r := newRig(t, signedOut)
	r.click("Paste a Music User Token")
	r.typeInto("Music User Token", strings.Repeat("x", 30))
	r.status(services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	r.status(signedOut)
	r.shows("Paste a Music User Token")
	r.hides("Use this token")
	if r.sec.flow.token != "" {
		t.Error("the token outlived its form")
	}
}

func TestTokenCancelForgetsTheToken(t *testing.T) {
	r := newRig(t, signedOut)
	r.click("Paste a Music User Token")
	r.typeInto("Music User Token", strings.Repeat("x", 30))
	r.click("Cancel")
	r.click("Paste a Music User Token")
	if r.sec.flow.token != "" || r.sec.flow.tokenError != "" {
		t.Errorf("Cancel kept the token %q / error %q", r.sec.flow.token, r.sec.flow.tokenError)
	}
}

func TestResetForgetsWhatWasTyped(t *testing.T) {
	r := newRig(t, signedOut)
	r.typeInto("Password", "hunter2")
	r.sec.Reset()
	r.tt.Frame()
	if r.sec.flow.password != "" || r.sec.flow.account != "" {
		t.Error("Reset kept the credentials")
	}
}

// Enter in the password field submits; the field is keyed anew, and the
// focus stays in it so the retyped password does not land elsewhere.
func TestEnterInPasswordKeepsTheFocusThereAfterAFailedAttempt(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, appleid.ErrInvalidCredentials }
	r.typeInto("Apple Account", "a@example.com")
	r.typeInto("Password", "pw")
	r.tt.Key(0, ui.KeyEnter)
	r.waitCalls("password", 1)
	r.tt.Until("the error", r.tt.Shows("That Apple Account or password isn't right"))
	for range 3 {
		r.tt.Advance(60 * time.Millisecond)
		r.tt.Drain()
	}
	if !r.tt.Focused("Password") {
		t.Errorf("the focus left the password field; texts %q", r.tt.Texts())
	}
}

func TestPasswordFieldIsANewFieldAfterEachAttempt(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, appleid.ErrInvalidCredentials }
	r.signIn("a@example.com", "pw")
	r.tt.Until("the error", r.tt.Shows("That Apple Account or password isn't right"))
	if r.sec.flow.attempt != 1 || r.sec.flow.password != "" {
		t.Errorf("attempt %d, password %q", r.sec.flow.attempt, r.sec.flow.password)
	}
}

func TestLockedAccountOffersTheBrowserAndTheUnlockPage(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, appleid.ErrLocked }
	r.signIn("a@example.com", "pw")
	r.tt.Until("locked", r.tt.Shows("This Apple Account is locked"))
	r.shows(
		"Apple has locked the account or made it temporarily unavailable. Unlock it at iforgot.apple.com, or sign in with your browser.",
		"Unlock at iforgot.apple.com",
	)
	r.click("Unlock at iforgot.apple.com")
	if got := r.h.Opened(); len(got) != 1 || got[0] != "https://iforgot.apple.com" {
		t.Errorf("opened %q", got)
	}
	// The notice's button is the first "Sign in with your browser".
	r.click("Sign in with your browser")
	r.waitCalls("signin", 1)
}

func TestActionRequiredOffersTheAccountSite(t *testing.T) {
	r := newRig(t, signedOut)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, appleid.ErrActionRequired }
	r.signIn("a@example.com", "pw")
	r.tt.Until("notice", r.tt.Shows("Your Apple Account needs attention"))
	r.click("account.apple.com")
	if got := r.h.Opened(); len(got) != 1 || got[0] != "https://account.apple.com" {
		t.Errorf("opened %q", got)
	}
}

func TestEverySignInErrorWordsItself(t *testing.T) {
	cases := map[error]string{
		appleid.ErrNoSubscription:     "Apple Music subscription required",
		appleid.ErrUnsupportedCountry: "Apple Music isn't available in this account's country",
		appleid.ErrUnsupported2FA:     "This account needs your browser to sign in",
		auth.ErrKeyring:               "Sona couldn't save your sign-in",
		errors.New("something else"):  "Sign-in didn't work",
	}
	for cause, title := range cases {
		r := newRig(t, signedOut)
		r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, cause }
		r.signIn("a@example.com", "pw")
		r.tt.Until(title, r.tt.Shows(title))
	}
}

func TestStatusNotices(t *testing.T) {
	for name, tc := range map[string]struct {
		st    services.AuthStatus
		shows []string
		hides []string
	}{
		"needs reauth": {
			services.AuthStatus{State: "needs_reauth"},
			[]string{"Apple stopped accepting Sona's sign-in", "This happens after a password change or when the token expires. Your library on this computer is still here; sign in again to keep syncing and playing.", "Apple Account"},
			nil,
		},
		"error": {
			services.AuthStatus{State: "error"},
			[]string{"Apple Music isn't accepting Sona's requests", "Sona refetches its developer token by itself; if this persists, Apple changed its web player.", "Apple Account"},
			nil,
		},
		"offline": {
			services.AuthStatus{State: "signed_out", Offline: true},
			[]string{"You're offline. Signing in needs a connection to Apple.", "Apple Account"},
			nil,
		},
		"restoring": {
			services.AuthStatus{State: "restoring"},
			nil,
			[]string{"Apple Account", "Signing in…", "Try again"},
		},
		"restoring, keyring waiting": {
			services.AuthStatus{State: "restoring", Keyring: "waiting"},
			[]string{"Signing in…", "Sona is waiting for your keyring to hand over your sign-in. If GNOME Keyring or KWallet asks you to unlock it, do that now."},
			[]string{"Apple Account", "Try again"},
		},
		"keyring unavailable": {
			services.AuthStatus{State: "signed_out", Keyring: "unavailable"},
			[]string{"Sona couldn't read your sign-in from your keyring", "Your keyring didn't answer or refused, so you're signed out. Unlock GNOME Keyring or KWallet, then try again.", "Try again", "Apple Account"},
			nil,
		},
		"no keyring": {
			services.AuthStatus{State: "signed_out", Unencrypted: true, Backend: "file"},
			[]string{"Signing in now stores your token unencrypted", "Sona couldn't open a keyring, so it would keep your Apple Music token in a file only your user can read. Start or unlock GNOME Keyring or KWallet, then try again.", "Try again", "Apple Account"},
			nil,
		},
		"plain signed out": {
			signedOut,
			[]string{"Apple Account"},
			[]string{"Try again", "Apple stopped accepting Sona's sign-in", "You're offline. Signing in needs a connection to Apple."},
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, tc.st)
			r.shows(tc.shows...)
			r.hides(tc.hides...)
		})
	}
}

// The notices about the keyring name this OS's: the Keychain on macOS,
// Credential Manager on Windows, never GNOME Keyring or KWallet there.
func TestKeyringNoticesAreWordedForTheOS(t *testing.T) {
	waiting := services.AuthStatus{State: "restoring", Keyring: "waiting"}
	refused := services.AuthStatus{State: "signed_out", Keyring: "unavailable"}
	noKeyring := services.AuthStatus{State: "signed_out", Unencrypted: true, Backend: "file"}
	inFile := services.AuthStatus{State: "signed_in", Unencrypted: true, Backend: "file", StorefrontID: "gb"}
	for _, tc := range []struct {
		goos string
		st   services.AuthStatus
		want string
	}{
		{"darwin", waiting, "If macOS asks you to unlock it, do that now."},
		{"darwin", refused, "Unlock your login keychain in Keychain Access, then try again."},
		{"darwin", noKeyring, "Sona couldn't open your Keychain, so it would keep your Apple Music token in a file only your user can read."},
		{"darwin", inFile, "Sona couldn't use your Keychain, so it keeps your Apple Music token in a file only your user can read."},
		{"windows", waiting, "Sona is waiting for Windows Credential Manager to hand over your sign-in."},
		{"windows", refused, "Windows Credential Manager didn't answer or refused, so you're signed out. Try again."},
		{"windows", noKeyring, "Sona couldn't open Windows Credential Manager, so it would keep your Apple Music token in a file only your user can read."},
		{"windows", inFile, "Sona couldn't use Windows Credential Manager, so it keeps your Apple Music token in a file only your user can read."},
		{"linux", refused, "Unlock GNOME Keyring or KWallet, then try again."},
	} {
		t.Run(tc.goos+"/"+tc.st.State+"/"+tc.st.Keyring, func(t *testing.T) {
			r := buildRig(t, rigOpts{status: &tc.st, os: tc.goos})
			r.shows(tc.want)
			if tc.goos != "linux" && (r.tt.HasText("GNOME") || r.tt.HasText("KWallet") || r.tt.HasText("Secret Service")) {
				t.Errorf("a Linux keyring on %s: %q", tc.goos, r.tt.Texts())
			}
		})
	}
}

func TestKeyringRetryReadsAgainAndStaysAsTrying(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", Keyring: "unavailable"})
	r.click("Try again")
	r.waitCalls("retry", 1)
	// The read runs: the button stays in place as "Trying…" and takes no click.
	r.status(services.AuthStatus{State: "restoring", Keyring: "waiting"})
	r.shows("Trying…")
	r.click("Trying…")
	r.tt.Drain()
	if n := r.api.count("retry"); n != 1 {
		t.Errorf("%d retries", n)
	}
	// It reads nothing: the notice is back with its button.
	r.status(services.AuthStatus{State: "signed_out", Keyring: "unavailable"})
	r.shows("Try again")
}

func TestKeyringRetryShowsNothingDuringTheStartupRead(t *testing.T) {
	h := uitest.New(t, uitest.Bare())
	api := &fakeAPI{}
	retry := NewKeyringRetry(Deps{API: api, Queries: h.Env.Queries, Post: h.Env.Post})
	st := services.AuthStatus{State: "restoring"}
	tt := h.Tester(t, func(c *ui.Context) { retry.Build(c, st) }, 300, 100)
	tt.Frame()
	if len(tt.Texts()) != 0 {
		t.Errorf("shown during the start-up read: %q", tt.Texts())
	}
	st = services.AuthStatus{State: "signed_out", Keyring: "unavailable"}
	tt.Frame()
	if !tt.HasText("Try again") {
		t.Fatalf("no button: %q", tt.Texts())
	}
	_ = tt.Click("Try again")
	tt.Until("retry", func() bool { return api.count("retry") == 1 })
}

func TestStatusErrorShowsAnErrorBlockWithRetry(t *testing.T) {
	// No service: the query fails, and Try again asks again.
	r := buildRig(t, rigOpts{noAPI: true})
	r.tt.Until("error", r.tt.Shows("Couldn't read the sign-in state"))
	r.shows("Try again")
	r.hides("Apple Account")
}

func TestLoadingShowsABusyPlaceholder(t *testing.T) {
	gate := make(chan struct{})
	r := buildRig(t, rigOpts{gate: gate, fetch: signedOut})
	r.tt.Frame()
	if _, ok := r.tt.Find("Loading"); !ok {
		t.Errorf("no loading placeholder: %q", r.tt.Texts())
	}
	r.hides("Apple Account", "Signed in to Apple Music")
	close(gate)
	r.tt.Until("status", r.tt.Shows("Use the Apple Account with your Apple Music subscription."))
}

var signedIn = services.AuthStatus{
	State: "signed_in", StorefrontID: "gb", StorefrontName: "United Kingdom", Language: "en-GB", Backend: "secret-service", Keyring: "",
}

func TestSignedInShowsTheAccount(t *testing.T) {
	r := newRig(t, signedIn)
	r.shows(
		"Signed in to Apple Music", "Storefront: United Kingdom · British English", "Sign out",
		"Where your token is kept", "secret-service",
	)
	r.hides("Apple Account", "Your token is stored unencrypted", "Explicit content is restricted on this account; Sona hides it.")

	validating := signedIn
	validating.State = "validating"
	r.status(validating)
	r.shows("Checking your sign-in…")

	waiting := signedIn
	waiting.Keyring = "waiting"
	r.status(waiting)
	r.shows("Saving your sign-in in your keyring; unlock it if it asks.")

	unknown := services.AuthStatus{State: "signed_in", StorefrontID: "gb"}
	r.status(unknown)
	r.shows("Storefront: GB", "Unknown")
	r.status(services.AuthStatus{State: "signed_in"})
	r.shows("Storefront unknown until Apple confirms the token")
}

func TestSignedInNotices(t *testing.T) {
	restricted := signedIn
	restricted.Restricted = true
	r := newRig(t, restricted)
	r.shows("Explicit content is restricted on this account; Sona hides it.")

	unencrypted := signedIn
	unencrypted.Unencrypted = true
	r.status(unencrypted)
	r.shows(
		"Your token is stored unencrypted",
		"No Secret Service keyring is running, so Sona keeps your Apple Music token in a file only your user can read. Start GNOME Keyring or KWallet and Sona moves it into the keyring.",
	)
	r.hides("Where your token is kept")
}

func TestSignedInOfflineChecksNow(t *testing.T) {
	offline := signedIn
	offline.Offline = true
	r := newRig(t, offline)
	r.shows("Sona couldn't reach Apple to confirm your sign-in. It keeps using your saved token, and retries by itself.", "Check now")
	r.api.onRevalid = func() error { return &net.DNSError{Err: "no such host"} }
	r.click("Check now")
	r.waitCalls("revalidate", 1)
	r.tt.Until("toast", r.tt.Shows("Couldn't reach Apple", "Sona couldn't reach the network. Check your connection and try again."))
}

func TestSignOutAsksThenSignsOut(t *testing.T) {
	r := newRig(t, signedIn)
	gate := make(chan struct{})
	r.api.onSignOut = func() error {
		<-gate
		r.api.mu.Lock()
		r.api.status = signedOut
		r.api.mu.Unlock()
		store.Set(r.h.Env.Queries, store.KeyAuth, signedOut)
		return nil
	}
	r.click("Sign out")
	r.shows("Sign out of Apple Music?", "Sona removes your token from the keyring, stops playback and deletes the copy of your library on this computer. Your Apple Music library itself isn't changed.", "Cancel")
	if _, ok := r.tt.Find("Close"); ok {
		t.Error("the confirm dialog has a close button")
	}
	if n := r.api.count("signout"); n != 0 {
		t.Fatalf("signed out before the answer: %d", n)
	}
	r.tt.Key(0, ui.KeyEnter) // the confirm button has the focus
	r.waitCalls("signout", 1)
	r.tt.Drain()
	r.shows("Signing out…", "Sign out of Apple Music?")
	close(gate)
	r.tt.Until("signed out", func() bool { return !r.tt.HasText("Sign out of Apple Music?") && r.tt.HasText("Apple Account") })
	if n := r.api.count("signout"); n != 1 {
		t.Errorf("%d sign-outs", n)
	}
}

func TestSignOutCancelAndEscapeDoNotSignOut(t *testing.T) {
	r := newRig(t, signedIn)
	r.click("Sign out")
	r.click("Cancel")
	r.hides("Sign out of Apple Music?")
	r.click("Sign out")
	r.tt.Key(0, ui.KeyEscape)
	r.hides("Sign out of Apple Music?")
	r.tt.Drain()
	if n := r.api.count("signout"); n != 0 {
		t.Errorf("%d sign-outs", n)
	}
	// The menu's "Sign out" asks the same question.
	r.sec.ConfirmSignOut()
	r.tt.Drain()
	r.shows("Sign out of Apple Music?")
}

func TestSignOutFailureKeepsTheDialogAndToasts(t *testing.T) {
	r := newRig(t, signedIn)
	r.api.onSignOut = func() error { return auth.ErrSignOutIncomplete }
	r.click("Sign out")
	r.tt.Key(0, ui.KeyEnter)
	r.waitCalls("signout", 1)
	r.tt.Until("toast", r.tt.Shows("Couldn't sign out", i18n.ErrorText(services.CodeSignOutIncomplete)))
	// The dialog is still there, asking again, and Enter retries.
	r.shows("Sign out of Apple Music?")
	r.hides("Signing out…")
	r.api.onSignOut = nil
	r.tt.Key(0, ui.KeyEnter)
	r.waitCalls("signout", 2)
}

func TestSignedInStatusReplacesTheFormAndSignedOutResetsIt(t *testing.T) {
	r := newRig(t, signedOut)
	r.typeInto("Apple Account", "a@example.com")
	r.status(signedIn)
	r.shows("Signed in to Apple Music")
	r.hides("Apple Account")
	r.status(signedOut)
	r.shows("Apple Account")
	if r.sec.flow.account != "" {
		t.Errorf("the old form came back: %+v", r.sec.flow)
	}
}

func TestFocusAccountFocusesTheFieldOnce(t *testing.T) {
	r := newRig(t, signedOut)
	r.sec.FocusAccount()
	r.tt.Frame()
	if !r.tt.Focused("Apple Account") {
		t.Fatalf("the Apple Account field is not focused")
	}
	// Once: the focus moves on when the user does.
	r.click("Password")
	r.tt.Frame()
	if r.tt.Focused("Apple Account") {
		t.Error("the field took the focus back")
	}
}

func TestFocusAccountIsDroppedWhenThereIsNoForm(t *testing.T) {
	r := newRig(t, signedIn)
	r.sec.FocusAccount()
	r.tt.Frame()
	r.status(signedOut)
	if r.tt.Focused("Apple Account") {
		t.Error("a request made while signed in focused the form later")
	}
}

func TestNoPasswordCodeOrTokenInLogsOrLinks(t *testing.T) {
	// The fake API answers with the secrets in its own record only; what the
	// views log, open or copy never carries them.
	var logs recordingHandler
	r := newRig(t, signedOut)
	r.sec.d.Log = newLogger(&logs)
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return deviceFactor, nil }
	r.signIn("a@example.com", "pw-secret-9")
	r.tt.Until("code step", r.tt.Shows("Enter the verification code"))
	r.typeInto("Code", "424242")
	r.waitCalls("submit", 1)
	r.tt.Until("done", r.tt.Shows("Checking your account with Apple…"))
	for _, s := range append(logs.all(), append(r.h.Opened(), r.h.Copied()...)...) {
		if strings.Contains(s, "pw-secret-9") || strings.Contains(s, "424242") {
			t.Errorf("a secret reached %q", s)
		}
	}
}

// focusSwitch tabs to the switch named label (the title beside it shares the
// text, so a click by name would land on the title).
func (r *rig) focusSwitch(label string) {
	r.t.Helper()
	for range 300 {
		if r.tt.Focused(label) {
			return
		}
		r.tt.Key(0, ui.KeyTab)
	}
	r.t.Fatalf("Tab never reached %q; texts %q", label, r.tt.Texts())
}

func TestTouchIDOfferedOnlyWhereTheBuildCanKeepItBehindTouchID(t *testing.T) {
	const title = "Use Touch ID to unlock Sona"
	available := signedIn
	available.BiometricAvailable = true
	r := buildRig(t, rigOpts{status: &available, os: "darwin"})
	r.shows(title)
	r.focusSwitch(title)
	r.tt.Key(0, ui.KeySpace)
	r.waitCalls("touchid true", 1)

	// Turned on: the status says so, and Space turns it off again.
	available.BiometricUnlock, available.Backend = true, "keychain (Touch ID)"
	r.status(available)
	r.shows("keychain (Touch ID)")
	r.focusSwitch(title)
	r.tt.Key(0, ui.KeySpace)
	r.waitCalls("touchid false", 1)
}

func TestTouchIDSwitchFailureSaysWhy(t *testing.T) {
	available := signedIn
	available.BiometricAvailable = true
	r := buildRig(t, rigOpts{status: &available, os: "darwin"})
	r.api.onTouchID = func(bool) error { return errors.New("keychain says no") }
	r.focusSwitch("Use Touch ID to unlock Sona")
	r.tt.Key(0, ui.KeySpace)
	r.waitCalls("touchid true", 1)
	r.tt.Until("toast", r.tt.Shows("Couldn't change Touch ID unlock"))
}

func TestTouchIDUnavailableOnAMacExplainsTheRequirementInsteadOfASwitch(t *testing.T) {
	r := buildRig(t, rigOpts{status: &signedIn, os: "darwin"})
	r.shows(
		"Touch ID unlock isn't available in this build",
		"It needs a Sona signed with an Apple Developer ID, with a team ID and the keychain-access-groups entitlement. This build is signed ad hoc, so your token stays in your login Keychain.",
	)
	r.hides("Use Touch ID to unlock Sona")
}

func TestTouchIDIsAbsentOffTheMac(t *testing.T) {
	r := newRig(t, signedIn)
	r.hides("Use Touch ID to unlock Sona", "Touch ID unlock isn't available in this build")
}

func TestTouchIDCancelledOffersTryAgain(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", Keyring: "locked"})
	r.shows("Touch ID didn't unlock Sona", "Try Touch ID again")
	r.click("Try Touch ID again")
	r.waitCalls("retry", 1)
}

func TestTouchIDResetTellsWhySignInIsNeeded(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", BiometricReset: true})
	r.shows("Touch ID unlock ended", "Sign in")
}
