package account

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// walkFrom is the Tab walk of the rig, rotated to start at the stop named
// first (the focus may start anywhere, on an autofocused field), as
// "role label" strings.
func walkFrom(t *testing.T, r *rig, first string) []string {
	t.Helper()
	r.tt.Drain()
	r.tt.Frame()
	access.NoNestedControls(t, r.tt.Tester)
	var names []string
	for _, s := range access.TabWalk(t, r.tt.Tester, 40) {
		names = append(names, s.Node.Label)
	}
	i := slices.Index(names, first)
	if i < 0 {
		t.Fatalf("the Tab walk %q never reaches %q", names, first)
	}
	return append(slices.Clone(names[i:]), names[:i]...)
}

// inOrder fails unless want is a subsequence of got.
func inOrder(t *testing.T, got, want []string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i < len(want) {
		t.Errorf("Tab walk %q lacks %q in that order", got, want)
	}
}

// TestKeyboardWalk is every view the account section shows: Tab reaches the
// primary controls in reading order, nothing nests a control in a control,
// and the walk comes back round (no trap).
func TestKeyboardWalk(t *testing.T) {
	tests := []struct {
		name  string
		build func(t *testing.T) *rig
		want  []string
	}{
		{"credentials", func(t *testing.T) *rig { return newRig(t, signedOut) },
			[]string{"Apple Account", "Password", "Sign in with your browser", "Paste a Music User Token"}},
		{"token paste", func(t *testing.T) *rig {
			r := newRig(t, signedOut)
			r.click("Paste a Music User Token")
			return r
		}, []string{"Apple Account", "Password", "Sign in with your browser", "Music User Token", "Cancel"}},
		{"browser waiting", func(t *testing.T) *rig {
			return newRig(t, services.AuthStatus{State: "signing_in", BrowserSignIn: true})
		}, []string{"Open the page again", "Copy link", "Cancel"}},
		{"code", func(t *testing.T) *rig {
			return newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
		}, []string{"Apple device", "Code", "Send a new code", "Cancel"}},
		{"phone choice", func(t *testing.T) *rig {
			return newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: phonesOnly})
		}, []string{"Text message", testPhones[0].Number, "Text me", "Cancel"}},
		{"locked", func(t *testing.T) *rig {
			r := newRig(t, signedOut)
			r.api.onPassword = func(string, string) (*services.SecondFactor, error) { return nil, appleid.ErrLocked }
			r.signIn("a@example.com", "pw")
			r.tt.Until("locked", r.tt.Shows("This Apple Account is locked"))
			return r
		}, []string{"Unlock at iforgot.apple.com", "Sign in with your browser", "Apple Account", "Password"}},
		{"signed in", func(t *testing.T) *rig { return newRig(t, signedIn) }, []string{"Sign out"}},
		{"signed in offline", func(t *testing.T) *rig {
			st := signedIn
			st.Offline = true
			return newRig(t, st)
		}, []string{"Check now", "Sign out"}},
		{"keyring unavailable", func(t *testing.T) *rig {
			return newRig(t, services.AuthStatus{State: "signed_out", Keyring: "unavailable"})
		}, []string{"Try again", "Apple Account", "Password"}},
		{"status error", func(t *testing.T) *rig {
			r := buildRig(t, rigOpts{noAPI: true})
			r.tt.Until("error", r.tt.Shows("Couldn't read the sign-in state"))
			return r
		}, []string{"Try again"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.build(t)
			inOrder(t, walkFrom(t, r, tc.want[0]), tc.want)
		})
	}
}

// TestControlsAreOnTheTree: every control of the sign-in views has the role
// and the name its visible text gives it, the form's fields carry their
// hints, and the flow's titles are level-3 headings.
func TestControlsAreOnTheTree(t *testing.T) {
	r := newRig(t, signedOut)
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	for _, name := range []string{"Apple Account", "Password"} {
		if _, ok := tr.Find(ui.RoleTextField, name); !ok {
			t.Errorf("no text field %q:\n%s", name, tr.Dump())
		}
	}
	if n, ok := tr.Find(ui.RoleTextField, "Apple Account"); !ok || n.Description != "The email address or phone number you use with Apple." {
		t.Errorf("the account field's hint: %+v", n)
	}
	if n, ok := tr.Find(ui.RoleTextField, "Password"); !ok || !n.Has(access.Password) {
		t.Errorf("the password field is not marked as a password: %+v", n)
	}
	if n, ok := tr.Find(ui.RoleHeading, "Sign in to Apple Music"); !ok || n.Level != 3 {
		t.Errorf("the form's title: %+v", n)
	}
	for _, name := range []string{"Sign in with your browser", "Paste a Music User Token"} {
		if _, ok := tr.Find(ui.RoleButton, name); !ok {
			t.Errorf("no button %q:\n%s", name, tr.Dump())
		}
	}
	// The submit is a real disabled button while the fields are empty.
	if n, ok := tr.Find(ui.RoleButton, "Sign in"); !ok || !n.Has(access.Disabled) {
		t.Errorf("the empty form's Sign in is not disabled: %+v", n)
	}
	// No spinner is a nameless progress node.
	for _, n := range tr.Nodes {
		if n.Role == ui.RoleProgress && n.Label == "" {
			t.Errorf("a nameless progress node: %s", tr.Dump())
		}
	}

	r = newRig(t, services.AuthStatus{State: "signing_in", BrowserSignIn: true})
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleHeading, "Finish signing in in your browser"); !ok {
		t.Errorf("no heading:\n%s", tr.Dump())
	}
	for _, n := range tr.Nodes {
		if n.Role == ui.RoleProgress && n.Label == "" {
			t.Errorf("a nameless progress node while waiting for the browser:\n%s", tr.Dump())
		}
	}
}

// TestKeyringRetryIsAriaDisabledWhileTrying: v0.2.0's aria-disabled. The
// button stays enabled and focusable (the focus stays on it), ignores the
// press and explains itself through its description.
func TestKeyringRetryIsAriaDisabledWhileTrying(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signed_out", Keyring: "unavailable"})
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleButton, "Try again")
	if !ok || n.Has(access.Disabled) || n.Description != "" {
		t.Fatalf("Try again at rest: %+v", n)
	}
	r.click("Try again")
	r.waitCalls("retry", 1)
	r.status(services.AuthStatus{State: "restoring", Keyring: "waiting"})
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	n, ok = tr.Find(ui.RoleButton, "Trying…")
	if !ok {
		t.Fatalf("no Trying… button:\n%s", tr.Dump())
	}
	if n.Has(access.Disabled) || !n.Has(access.Focusable) {
		t.Errorf("Trying… must stay focusable and enabled: %v", n.States)
	}
	if !strings.Contains(n.Description, "waiting for your keyring") {
		t.Errorf("Trying… does not say why: %q", n.Description)
	}
	if !r.tt.Focused("Trying…") {
		t.Error("the focus did not stay on the button while it reads again")
	}
	access.NoNestedControls(t, r.tt.Tester)
}

// TestBusyStepsAreAnnounced: aria-busy has no MyGo equivalent, so a busy
// step says what it is doing once, as it starts; and its failure is read
// out by the notice.
func TestBusyStepsAreAnnounced(t *testing.T) {
	r := newRig(t, signedOut)
	gate := make(chan struct{})
	r.api.onPassword = func(string, string) (*services.SecondFactor, error) {
		<-gate
		return nil, appleid.ErrInvalidCredentials
	}
	r.typeInto("Apple Account", "a@example.com")
	r.typeInto("Password", "pw")
	r.tt.Announcements()
	r.click("Sign in")
	r.waitCalls("password", 1)
	r.tt.Frame()
	if got := r.tt.Announcements(); !slices.Contains(got, "Signing in…") {
		t.Errorf("the busy sign-in was not announced: %q", got)
	}
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleButton, "Signing in…"); !ok || !n.Has(access.Disabled) {
		t.Errorf("the busy submit: %+v", n)
	}
	r.tt.Frame()
	if got := r.tt.Announcements(); slices.Contains(got, "Signing in…") {
		t.Errorf("the busy sign-in was announced twice: %q", got)
	}
	close(gate)
	r.tt.Until("error", r.tt.Shows("That Apple Account or password isn't right"))
	if got := r.tt.Announcements(); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, "That Apple Account or password isn't right") }) {
		t.Errorf("the failure was not announced: %q", got)
	}
	tr = access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "That Apple Account or password isn't right Check the email or phone number and the password, then try again."); !ok {
		t.Errorf("the error is not a status:\n%s", tr.Dump())
	}
}

// TestTokenCheckIsAnnounced is the token paste's busy step and its error.
func TestTokenCheckIsAnnounced(t *testing.T) {
	r := newRig(t, signedOut)
	gate := make(chan struct{})
	r.api.onToken = func(string) error { <-gate; return nil }
	r.click("Paste a Music User Token")
	r.sec.flow.token = strings.Repeat("t", 30)
	r.tt.Drain()
	r.tt.Announcements()
	r.click("Use this token")
	r.waitCalls("token", 1)
	r.tt.Frame()
	if got := r.tt.Announcements(); !slices.Contains(got, "Checking…") {
		t.Errorf("the token check was not announced: %q", got)
	}
	close(gate)
}

// TestDoneStepIsAStatus: the wait for Go's status after an accepted sign-in
// is a live status, named by its text.
func TestDoneStepIsAStatus(t *testing.T) {
	r := newRig(t, signedOut)
	r.signIn("a@example.com", "pw")
	r.tt.Until("done", r.tt.Shows("Checking your account with Apple…"))
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Checking your account with Apple…"); !ok {
		t.Errorf("not a status:\n%s", tr.Dump())
	}
}

// TestLoadingPlaceholderIsABusyStatus is aria-busy of the 12rem placeholder.
func TestLoadingPlaceholderIsABusyStatus(t *testing.T) {
	gate := make(chan struct{})
	r := buildRig(t, rigOpts{gate: gate, fetch: signedOut})
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("no Loading status:\n%s", tr.Dump())
	}
	close(gate)
}

// TestSignOutDialogTakesTheFocusAndEscapeGivesItBack: the confirm is an
// alert dialog whose buttons Tab reaches; Escape closes it and the focus
// returns to the Sign out button that opened it.
func TestSignOutDialogTakesTheFocusAndEscapeGivesItBack(t *testing.T) {
	r := newRig(t, signedIn)
	r.tt.Frame()
	r.tt.SetFocused(true)
	r.tt.Key(0, ui.KeyTab) // the page's one control, as a keyboard user reaches it
	if !r.tt.Focused("Sign out") {
		t.Fatalf("Tab did not reach Sign out; texts %q", r.tt.Texts())
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Drain()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	d, ok := tr.Find(ui.RoleAlertDialog, "Sign out of Apple Music?")
	if !ok {
		t.Fatalf("no alert dialog:\n%s", tr.Dump())
	}
	for _, name := range []string{"Cancel", "Sign out"} {
		n, ok := tr.Find(ui.RoleButton, name)
		if !ok || !tr.Within(n, ui.RoleAlertDialog) {
			t.Errorf("button %q is not inside the dialog %v", name, d)
		}
	}
	if !r.tt.Focused("Sign out") {
		t.Error("the focus is not in the dialog")
	}
	access.NoNestedControls(t, r.tt.Tester)
	if stops := access.TabWalk(t, r.tt.Tester, 10); len(stops) != 2 {
		t.Errorf("the dialog traps Tab on %d stops, want its two buttons", len(stops))
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Drain()
	r.hides("Sign out of Apple Music?")
	if !r.tt.Focused("Sign out") {
		t.Errorf("the focus did not return to the Sign out button; texts %q", r.tt.Texts())
	}
	tr = access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleButton || tr.Focus.Label != "Sign out" {
		t.Errorf("focused node %+v", tr.Focus)
	}
	kit.CancelConfirm()
}

// TestStatusNoticesThatAppearAreAnnounced: v0.2.0's notices were polite live
// regions. A notice there when the section is first built is read with the
// dialog; one that appears after is announced once.
func TestStatusNoticesThatAppearAreAnnounced(t *testing.T) {
	r := newRig(t, signedIn)
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("announced at rest: %q", got)
	}
	offline := signedIn
	offline.Offline = true
	r.status(offline)
	r.tt.Frame()
	got := r.tt.Announcements()
	if len(got) != 1 || !strings.Contains(got[0], "Sona couldn't reach Apple to confirm your sign-in") {
		t.Errorf("the offline notice: %q", got)
	}
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced again: %q", got)
	}
	r.status(signedIn)
	r.status(services.AuthStatus{State: "signed_out", Keyring: "unavailable"})
	r.tt.Frame()
	if got := r.tt.Announcements(); !slices.Contains(got, "Sona couldn't read your sign-in from your keyring") {
		t.Errorf("the keyring notice: %q", got)
	}
}

// TestCodeStepNoticeIsAnnounced is the code step's role=status line (the
// code was sent again, say).
func TestCodeStepNoticeIsAnnounced(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "signing_in", SecondFactor: deviceFactor})
	r.tt.Announcements()
	r.click("Send a new code")
	r.waitCalls("resend", 1)
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	var notice string
	for _, n := range tr.Roles(ui.RoleStatus) {
		notice = n.Label
	}
	if notice == "" {
		t.Fatalf("no status after sending a new code:\n%s", tr.Dump())
	}
	if got := r.tt.Announcements(); !slices.ContainsFunc(got, func(s string) bool { return strings.Contains(s, notice) }) {
		t.Errorf("%q was not announced: %q", notice, got)
	}
}
