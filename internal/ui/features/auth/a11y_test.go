package auth

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// TestMenuKeyboardWalk is the avatar and what it opens for each state of the
// account: the avatar is the page's one Tab stop, the menu is named and its
// rows are items in reading order, nothing nests a control in a control.
func TestMenuKeyboardWalk(t *testing.T) {
	offline := signedIn
	offline.Offline, offline.Restricted = true, true
	tests := []struct {
		name  string
		st    services.AuthStatus
		items []string
	}{
		{"signed out", signedOut, []string{"Sign in", "Private listening"}},
		{"signed in", signedIn, []string{"Account settings", "Private listening", "Sign out"}},
		{"offline", offline, []string{"Account settings", "Check now", "Private listening", "Sign out"}},
		{"keyring", services.AuthStatus{State: "signed_out", Keyring: "unavailable"}, []string{"Sign in", "Try again", "Private listening"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, tc.st)
			r.tt.Frame()
			access.NoNestedControls(t, r.tt.Tester)
			stops := access.TabWalk(t, r.tt.Tester, 6)
			if len(stops) != 1 || stops[0].Node.Role != ui.RoleButton || stops[0].Node.Label != "Account" {
				t.Fatalf("the closed top bar's stops: %v", stops)
			}
			r.openMenu()
			r.tt.Frame()
			tr := access.Read(t, r.tt.Tester)
			avatar, ok := tr.Find(ui.RoleButton, "Account")
			if !ok || !avatar.Has(access.Expanded) || avatar.HasPopup != ui.PopupMenu {
				t.Fatalf("the avatar does not say it opens a menu, open: %+v", avatar)
			}
			menu, ok := tr.Find(ui.RoleMenu, "Account")
			if !ok {
				t.Fatalf("no menu named Account:\n%s", tr.Dump())
			}
			var got []string
			for _, n := range tr.Children(menu) {
				if n.Role == ui.RoleMenuItem || n.Role == ui.RoleMenuItemCheckBox {
					got = append(got, n.Label)
				}
			}
			if !slices.Equal(got, tc.items) {
				t.Errorf("menu items %q, want %q", got, tc.items)
			}
			access.NoNestedControls(t, r.tt.Tester)
		})
	}
}

// TestAvatarSaysWhereTheAccountStands: the avatar's name is "Account", and
// its description is the state line (once, whether or not the menu is open),
// which is what aria-describedby gave it.
func TestAvatarSaysWhereTheAccountStands(t *testing.T) {
	r := newRig(t, signedIn)
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleButton, "Account")
	if !ok {
		t.Fatalf("no avatar button:\n%s", tr.Dump())
	}
	if got := strings.Count(n.Description, "Signed in · United Kingdom"); got != 1 {
		t.Errorf("the state line is read %d times in %q", got, n.Description)
	}
	r.openMenu()
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	n, _ = tr.Find(ui.RoleButton, "Account")
	if n.Description != "Signed in · United Kingdom" {
		t.Errorf("the open avatar's description %q", n.Description)
	}
}

// TestAttentionDotIsOnTheTreeThroughTheDescription: the red dot is only
// paint; what it means reaches assistive technology as the state line.
func TestAttentionDotIsOnTheTreeThroughTheDescription(t *testing.T) {
	r := newRig(t, services.AuthStatus{State: "needs_reauth"})
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleButton, "Account")
	if !ok || !strings.Contains(n.Description, "Apple stopped accepting Sona's sign-in") {
		t.Errorf("the avatar does not say the account needs attention: %+v", n)
	}
}

// TestPrivateListeningStates: a checkbox row with its state, off until Go has
// answered (a real disabled state), and explaining itself when forced.
func TestPrivateListeningStates(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleMenuItemCheckBox, "Private listening")
	if !ok || n.Has(access.Checked) || n.Has(access.Disabled) {
		t.Errorf("Private listening at rest: %+v", n)
	}

	store.Set(r.h.Env.Queries, store.KeyPrivate, services.PrivateState{On: true, Source: "environment"})
	r.settle()
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	n, ok = tr.Find(ui.RoleMenuItemCheckBox, "Private listening")
	if !ok || !n.Has(access.Checked) {
		t.Fatalf("a forced Private listening is not checked: %+v", n)
	}
	if !strings.Contains(n.Description, "SONA_PRIVATE=1") {
		t.Errorf("the forced row does not say why: %q", n.Description)
	}
}

// TestDialogKeyboardWalk is the account dialog: named, its titles headings,
// Tab stays inside, Escape closes it and gives the focus back.
func TestDialogKeyboardWalk(t *testing.T) {
	r := newRig(t, signedOut)
	r.openMenu()
	r.click("Sign in")
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleDialog, "Account"); !ok {
		t.Fatalf("no dialog named Account:\n%s", tr.Dump())
	}
	if h, ok := tr.Find(ui.RoleHeading, "Your Apple Music, on your desktop"); !ok || h.Level != 2 {
		t.Errorf("the dialog's heading: %+v", h)
	}
	if h, ok := tr.Find(ui.RoleHeading, "Sign in to Apple Music"); !ok || h.Level != 3 {
		t.Errorf("the form title: %+v", h)
	}
	access.NoNestedControls(t, r.tt.Tester)
	var names []string
	for _, s := range access.TabWalk(t, r.tt.Tester, 20) {
		names = append(names, s.Node.Label)
	}
	for _, want := range []string{"Apple Account", "Password", "Sign in with your browser", "Paste a Music User Token", "Close"} {
		if !slices.Contains(names, want) {
			t.Errorf("the dialog's Tab walk %q lacks %q", names, want)
		}
	}
	for _, n := range names {
		if n == "Account" {
			t.Errorf("Tab left the dialog for the page behind: %q", names)
		}
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.settle()
	if r.h.Env.Surfaces.AccountOpen() {
		t.Fatal("Escape did not close the dialog")
	}
}

// TestSignOutConfirmKeyboard: from the menu's Sign out the confirm is an
// alert dialog with two buttons; Escape answers no and the page is left
// with a Tab stop again.
func TestSignOutConfirmKeyboard(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.click("Sign out")
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleAlertDialog, "Sign out of Apple Music?"); !ok {
		t.Fatalf("no alert dialog:\n%s", tr.Dump())
	}
	access.NoNestedControls(t, r.tt.Tester)
	if stops := access.TabWalk(t, r.tt.Tester, 10); len(stops) != 2 {
		t.Errorf("the confirm's Tab stops: %v", stops)
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.settle()
	if r.tt.HasText("Sign out of Apple Music?") || r.api.count("SignOut") != 0 {
		t.Error("Escape did not cancel the confirm")
	}
}

// TestMenuEscapeReturnsFocusToTheAvatar is the menu's focus return as a
// tree fact.
func TestMenuEscapeReturnsFocusToTheAvatar(t *testing.T) {
	r := newRig(t, signedIn)
	r.openMenu()
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleButton || tr.Focus.Label != "Account" {
		t.Errorf("focus %+v", tr.Focus)
	}
	if n, _ := tr.Find(ui.RoleButton, "Account"); n.Has(access.Expanded) {
		t.Error("the avatar still says the menu is open")
	}
}

// TestDialogHasATitleSignedInAndStacksWhenNarrow: signed in the account's
// panel gives the view the heading the form brings itself, and in a window
// too narrow for two panels the lead sits above the account, whose controls
// are still in the dialog's Tab walk.
func TestDialogHasATitleSignedInAndStacksWhenNarrow(t *testing.T) {
	r := newRig(t, signedIn)
	r.h.Env.Surfaces.OpenAccount()
	r.settle()
	tr := access.Read(t, r.tt.Tester)
	if h, ok := tr.Find(ui.RoleHeading, "Account"); !ok || h.Level != 3 {
		t.Errorf("the signed-in view's title: %+v (found %v)", h, ok)
	}
	r.tt.Key(0, ui.KeyEscape)
	r.settle()

	n := newRig(t, signedOut)
	n.tt.SetSize(600, 700)
	n.h.Env.Surfaces.OpenAccount()
	n.settle()
	lead, ok := n.tt.Find("Your Apple Music, on your desktop")
	form, ok2 := n.tt.Find("Sign in to Apple Music")
	if !ok || !ok2 || form.Y <= lead.Y+lead.H {
		t.Errorf("the lead is at %v (found %v), the form at %v (found %v): want the form under the lead", lead, ok, form, ok2)
	}
	var names []string
	for _, s := range access.TabWalk(t, n.tt.Tester, 20) {
		names = append(names, s.Node.Label)
	}
	for _, want := range []string{"Apple Account", "Password", "Close"} {
		if !slices.Contains(names, want) {
			t.Errorf("the stacked dialog's Tab walk %q lacks %q", names, want)
		}
	}
}
