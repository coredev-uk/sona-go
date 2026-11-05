package account

import (
	"context"
	"reflect"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// viewKind is which of the flow's views is on screen; the fields of a view
// live as long as it does (React state of the component that unmounts).
type viewKind int

const (
	viewNone viewKind = iota
	viewCredentials
	viewBrowser
	viewCode
	viewPhone
	viewDone
)

// statusSig is the part of the auth status the form follows (the deps of
// SignInFlow's effect).
type statusSig struct {
	State            string
	Factor           *services.SecondFactor
	LastErrorCode    services.ErrorCode
	LastAppleMessage string
}

// flow is SignInFlow's state while it is mounted: the reducer's step, the
// text of the fields of the view shown, and the token paste form. A password,
// a code or a token lives here only while its view does.
type flow struct {
	step SignInStep
	seen *statusSig
	view viewKind

	account, password string        // credentials
	refocusPassword   bool          // the next password field takes the focus (it was just re-keyed)
	attempt           int           // password submissions so far: keys the field, so its undo history starts afresh
	code              string        // code
	codeFocus         kit.FocusStep // the code field takes the focus once its view is shown
	phoneID           int           // phone choice

	busyAnnounced        bool // this busy stretch was announced
	tokenAnnounced       bool // the token check's busy stretch was announced
	tokenOpen, tokenBusy bool
	token, tokenError    string
}

func (f *flow) apply(a SignInAction) { f.step = signInReducer(f.step, a) }

// enter makes v the view shown: the fields of the view that went away are
// dropped, those of the new one start fresh.
func (f *flow) enter(v viewKind) {
	if v == f.view {
		return
	}
	switch f.view {
	case viewCredentials:
		f.account, f.password = "", ""
	case viewCode:
		f.code = ""
	}
	if v == viewCode {
		f.codeFocus = 0
	}
	if v == viewPhone {
		f.phoneID = 0
		if fac := f.step.Factor; fac != nil && len(fac.Phones) > 0 {
			f.phoneID = fac.Phones[0].ID
		}
	}
	if v != viewCredentials {
		f.token, f.tokenOpen, f.tokenBusy, f.tokenError = "", false, false, ""
	}
	f.view = v
}

// Section is one mounted AccountSection (the account dialog's body, the
// setup's Apple Music step). Main thread only.
type Section struct {
	d     Deps
	retry *KeyringRetry
	flow  *flow

	focusAccount bool
	focusStep    kit.FocusStep // FocusAccount's focus, given once the form has been shown a frame
	credShown    bool          // the credentials form was built this frame

	// statusLine is the status notice the last Build showed, "" for none, and
	// statusKnown whether a Build has shown the section since Reset: a notice
	// that appears after that is read out (React's role=status live region).
	statusLine  string
	statusKnown bool

	checking bool // Revalidate runs

	signingOut bool // SignOut runs

	touchBusy bool // SetBiometricUnlock runs
}

// New is a Section over d.
func New(d Deps) *Section { return &Section{d: d, retry: NewKeyringRetry(d)} }

// Reset forgets the sign-in form: what was typed (a password, a code, a
// pasted token) and the step. The host calls it when it stops showing the
// section (the dialog closing), as React unmounted it; a pending challenge
// comes back from Go's status when the section is built again.
func (s *Section) Reset() {
	s.flow, s.focusAccount = nil, false
	s.statusLine, s.statusKnown = "", false
}

// FocusAccount asks the next Build to focus the Apple Account field once.
func (s *Section) FocusAccount() { s.focusAccount, s.focusStep = true, 0 }

// call runs a service call off the main thread and hands its outcome back.
func (s *Section) call(fn func() error, done func(err error)) {
	go func() {
		err := fn()
		s.d.Post(func() { done(err) })
	}()
}

func (s *Section) open(url string) {
	if s.d.OpenURL == nil {
		return
	}
	if err := s.d.OpenURL(url); err != nil && s.d.Log != nil {
		s.d.Log.Warn("account: couldn't open a page in the browser")
	}
}

// Build is AccountSection{actions}: the error block, the loading
// placeholder, the signed-in view, or the keyring notice, the state notices
// and the sign-in flow. hosted is React's `actions`: the host builds the
// Sign in submit itself (FootAction) and titles the form.
func (s *Section) Build(c *ui.Context, hosted bool) {
	s.credShown = false
	res := UseStatus(s.d)
	if res.Err != nil {
		s.flow = nil
		kit.ErrorBlock(c, i18n.T("Couldn't read the sign-in state"), res.Err, kit.ErrorBlockOpts{
			OnRetry:  func() { s.d.Queries.Invalidate(store.KeyAuth) },
			Retrying: res.Fetching,
		})
		return
	}
	if !res.HasData {
		s.flow = nil
		placeholder(c, i18n.T("Loading"))
		return
	}
	st := res.Data
	s.announceStatus(c, st)
	if HasAccount(st.State) {
		s.flow, s.focusAccount = nil, false
		s.signedIn(c, st)
		return
	}
	restoring := st.State == "restoring"
	ui.Column(c.Key("account")).FillWidth().Gap(16).Children(func() {
		s.keyringNotice(c.Key("keyring"), st)
		if restoring {
			s.flow = nil
			if st.Keyring != "waiting" {
				placeholder(c.Key("restoring"), i18n.T("Loading"))
			}
			return
		}
		if st.BiometricReset {
			notice(c.Key("touch-reset"), noticeOpts{Icon: "lock", IconColor: "danger"}, func() {
				noticeTitle(c, i18n.T("Touch ID unlock ended"))
				noticeDetail(c, i18n.T("Your enrolled fingerprints changed, so macOS locked your saved sign-in away. Sign in again, then turn Touch ID unlock back on."))
			}, nil)
		}
		if st.State == "needs_reauth" {
			notice(c.Key("reauth"), noticeOpts{Icon: "circle-alert", IconColor: "danger"}, func() {
				noticeTitle(c, i18n.T("Apple stopped accepting Sona's sign-in"))
				noticeDetail(c, i18n.T("This happens after a password change or when the token expires. Your library on this computer is still here; sign in again to keep syncing and playing."))
			}, nil)
		}
		if st.State == "error" {
			notice(c.Key("error"), noticeOpts{Icon: "circle-alert", IconColor: "danger"}, func() {
				noticeTitle(c, i18n.T("Apple Music isn't accepting Sona's requests"))
				noticeDetail(c, i18n.T("Sona refetches its developer token by itself; if this persists, Apple changed its web player."))
			}, nil)
		}
		if st.Offline {
			notice(c.Key("offline"), noticeOpts{Icon: "wifi-off", IconColor: "ink-muted"}, func() {
				text(c, i18n.T("You're offline. Signing in needs a connection to Apple."), "secondary", "ink")
			}, nil)
		}
		s.signInFlow(c.Key("flow"), st, hosted)
	})
}

// FootAction builds the hosted form's submit (Sign in / "Signing in…") and
// nothing when the credentials form is not the step shown.
func (s *Section) FootAction(c *ui.Context) {
	if !s.credShown || s.flow == nil {
		return
	}
	s.signInButton(c, s.flow, true)
}

// signInFlow is SignInFlow: the step the reducer is on, kept in step with
// Go's status.
func (s *Section) signInFlow(c *ui.Context, st services.AuthStatus, hosted bool) {
	f := s.flow
	if f == nil {
		f = &flow{step: initialSignIn}
		s.flow = f
	}
	// Keep the form in step with Go: restored challenges, timeouts, sign-in elsewhere.
	sig := statusSig{State: st.State, Factor: st.SecondFactor, LastErrorCode: st.LastErrorCode, LastAppleMessage: st.LastAppleMessage}
	if f.seen == nil || !reflect.DeepEqual(*f.seen, sig) {
		f.seen = &sig
		f.apply(SignInAction{
			Type: ActStatus, State: st.State, Factor: st.SecondFactor,
			LastErrorCode: st.LastErrorCode, LastAppleMessage: st.LastAppleMessage,
		})
	}
	step := f.step
	// A password exchange in flight is signing_in too, but it is not waiting for a browser.
	browserPending := st.State == "signing_in" && st.SecondFactor == nil && step.Kind == StepCredentials && !step.Busy
	switch {
	case browserPending:
		f.enter(viewBrowser)
	case step.Kind == StepCode:
		f.enter(viewCode)
	case step.Kind == StepChoosePhone:
		f.enter(viewPhone)
	case step.Kind == StepDone:
		f.enter(viewDone)
	default:
		f.enter(viewCredentials)
	}

	// A busy step is what aria-busy was: the button's label says so, and
	// MyGo has no busy state, so say it once as it starts.
	if step.Busy && !f.busyAnnounced {
		c.Announce(busyText(step))
	}
	f.busyAnnounced = step.Busy

	ui.Column(c).FillWidth().Children(func() {
		switch f.view {
		case viewBrowser:
			s.browserWaiting(c.Key("browser"), st)
		case viewCode:
			s.codeForm(c.Key("code"), f)
		case viewPhone:
			s.phoneChoice(c.Key("phone"), f)
		case viewDone:
			ui.Row(c.Key("done")).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c).Size(16, 16).Role(ui.RoleNone)
				text(c, i18n.T("Checking your account with Apple…"), "body", "ink-muted")
			}).Role(ui.RoleStatus)
		default:
			// A host with its own foot (the setup) titles the form itself.
			if !hosted {
				ui.Column(c.Key("heading")).FillWidth().Margin(0, 0, 20, 0).Children(func() {
					flowTitle(c, i18n.T("Sign in to Apple Music"))
					text(c, i18n.T("Use the Apple Account with your Apple Music subscription."), "body", "ink-muted").Margin(4, 0, 0, 0)
				})
			}
			s.credentialsForm(c.Key("credentials"), f, hosted)
		}
		if f.view == viewCredentials {
			s.alternatives(c.Key("alternatives"), f)
		}
	})
}

// alternatives is the part under the credentials form: the browser
// fallback and the token paste.
func (s *Section) alternatives(c *ui.Context, f *flow) {
	ui.Column(c).FillWidth().Margin(24, 0, 0, 0).Children(func() {
		hairline(c.Key("rule"))
		ui.Column(c.Key("body")).FillWidth().AlignItems(ui.Start).Gap(12).Padding(20, 0, 0, 0).Children(func() {
			text(c, i18n.T("Use a passkey, a security key or a browser you're already signed in to:"), "secondary", "ink-muted")
			if kit.Button(c.Key("browser"), i18n.T("Sign in with your browser"), kit.ButtonOpts{
				Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link", IconAfter: true, Disabled: f.step.Busy,
			}).Clicked() {
				s.browser(f)
			}
			s.tokenPaste(c.Key("token"), f)
		})
	})
}

// browser is the browser sign-in: it resets the form, then waits in
// AuthService.SignIn; "cancelled" is ignored, other failures show as errors.
func (s *Section) browser(f *flow) {
	f.apply(SignInAction{Type: ActCancel})
	api := s.d.API
	s.call(func() error { return api.SignIn(context.Background()) }, func(err error) {
		if err == nil || s.flow != f {
			return
		}
		if e := classifySignInError(err, "", ""); e.Code != services.CodeCancelled {
			f.apply(SignInAction{Type: ActFailed, Error: e})
		}
	})
}

// errorNotice is an error from the sign-in flow: what happened, and the
// button for the next step. onBrowser is nil where the browser is no step.
func (s *Section) errorNotice(c *ui.Context, e SignInError, onBrowser func()) {
	cp := errorCopy(e, s.d.OS)
	notice(c, noticeOpts{Icon: "circle-alert", IconColor: "danger", Problem: true, Announce: cp.Title + " " + cp.Detail}, func() {
		noticeTitle(c, cp.Title)
		noticeDetail(c, cp.Detail)
		if e.Code == services.CodeLocked {
			if kit.Button(c.Key("iforgot"), i18n.T("Unlock at iforgot.apple.com"), kit.ButtonOpts{Variant: kit.Link, Size: kit.SizeXS}).Clicked() {
				s.open("https://iforgot.apple.com")
			}
		}
	}, func() {
		switch {
		case cp.Next == NextBrowser && onBrowser != nil:
			if kit.Button(c.Key("next-browser"), i18n.T("Sign in with your browser"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Clicked() {
				onBrowser()
			}
		case cp.Next == NextAccountSite:
			if kit.Button(c.Key("next-site"), "account.apple.com", kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link"}).Clicked() {
				s.open("https://account.apple.com")
			}
		}
	})
}

// field is a labelled control with its hint under it (Account.tsx's Field):
// a click on the label focuses the control, which the label names.
func field(c *ui.Context, label, hint string, control func()) {
	e := ui.Field(c, label, control)
	if hint != "" {
		e.Description(hint)
	}
}

// fieldFace colours a text area as kit.Input's field.
func fieldFace(e ui.Element) {
	border := theme.C("input")
	if e.Focused() {
		border = theme.C("ring")
	}
	e.Radius(theme.Current().Radius["md"]).Background(theme.C("raised")).Border(1, border)
}

// trimmed is the account as the service gets it.
func trimmed(s string) string { return strings.TrimSpace(s) }

// busyText is what a busy step is doing, as its button words it.
func busyText(step SignInStep) string {
	switch step.Kind {
	case StepCode:
		return i18n.T("Checking…")
	case StepChoosePhone:
		return i18n.T("Sending…")
	}
	return i18n.T("Signing in…")
}

// statusNotice is the title of the status notice the account shows for st,
// "" for none: the line that is read out when it appears.
func statusNotice(st services.AuthStatus) string {
	if HasAccount(st.State) {
		switch {
		case st.Offline:
			return i18n.T("Sona couldn't reach Apple to confirm your sign-in. It keeps using your saved token, and retries by itself.")
		case st.Unencrypted:
			return i18n.T("Your token is stored unencrypted")
		}
		return ""
	}
	switch {
	case st.State == "restoring" && st.Keyring == "waiting":
		return i18n.T("Signing in…")
	case st.State == "restoring":
		return ""
	case st.Keyring == "unavailable":
		return i18n.T("Sona couldn't read your sign-in from your keyring")
	case st.Keyring == "locked":
		return i18n.T("Touch ID didn't unlock Sona")
	case st.BiometricReset:
		return i18n.T("Touch ID unlock ended; sign in again")
	case st.Unencrypted:
		return i18n.T("Signing in now stores your token unencrypted")
	case st.State == "needs_reauth":
		return i18n.T("Apple stopped accepting Sona's sign-in")
	case st.State == "error":
		return i18n.T("Apple Music isn't accepting Sona's requests")
	case st.Offline:
		return i18n.T("You're offline. Signing in needs a connection to Apple.")
	}
	return ""
}

// announceStatus reads out a status notice that appears while the section is
// shown, as v0.2.0's role=status (polite live) notices were; a notice that
// was there when the section was first built is read with the dialog.
func (s *Section) announceStatus(c *ui.Context, st services.AuthStatus) {
	line := statusNotice(st)
	if s.statusKnown && line != "" && line != s.statusLine {
		c.Announce(line)
	}
	s.statusLine, s.statusKnown = line, true
}
