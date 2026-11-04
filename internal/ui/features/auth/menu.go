package auth

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// avatarSize is the avatar's diameter (v0.2.0's size-7).
const avatarSize = 28

// stateLine is one line for where the account stands, under "Apple Music" in
// the menu, in the avatar's tooltip and as its description (useStateLine).
func stateLine(s services.AuthStatus) string {
	if account.HasAccount(s.State) {
		if s.Keyring == "waiting" {
			return i18n.T("Saving your sign-in in your keyring; unlock it if it asks.")
		}
		if s.State == "validating" {
			return i18n.T("Checking your sign-in…")
		}
		name := s.StorefrontName
		if name == "" {
			name = strings.ToUpper(s.StorefrontID)
		}
		if name != "" {
			return i18n.T("Signed in · {store}", "store", name)
		}
		return i18n.T("Signed in to Apple Music")
	}
	switch s.State {
	case "restoring", "signing_in":
		return i18n.T("Signing in…")
	case "needs_reauth":
		return i18n.T("Apple stopped accepting Sona's sign-in")
	case "error":
		return i18n.T("Apple Music isn't accepting Sona's requests")
	}
	if s.Keyring == "unavailable" {
		return i18n.T("Sona couldn't read your sign-in from your keyring")
	}
	return i18n.T("Not signed in")
}

// topBar is Slots.TopBarAccount: the avatar with its menu, and the account
// dialog the menu and every "Sign in" action open. They sit in a row of their
// own so the dialog's layer takes no room in the top bar.
func (f *feature) topBar(c *ui.Context) {
	ui.Row(c.Key("auth:slot")).AlignItems(ui.Center).Children(func() {
		f.accountMenu(c)
		f.dialog(c)
	})
}

// accountMenu is AccountMenu: the avatar opens a menu with where the Apple
// Music account stands (storefront, restricted content, offline, the
// keyring), signing in or the account's settings (the account dialog),
// private listening, and signing out. Enter, Space or a click open it; the
// arrows move; Escape closes it and gives the focus back to the avatar.
func (f *feature) accountMenu(c *ui.Context) {
	res := account.UseStatus(f.d)
	priv := store.UsePrivate(f.env.Queries, f.env.Core)
	known := res.HasData
	status := res.Data // zero until Go answers
	line := ""
	if known {
		line = stateLine(status)
	}
	avatar := f.avatar(c, account.HasAccount(status.State), account.NeedsAttention(status), f.menu.IsOpen())
	if avatar.Clicked() {
		if f.menu.IsOpen() {
			f.menu.Close()
		} else {
			f.menu.OpenFrom(avatar.FocusVisible())
		}
	}
	// The state line is the avatar's description and, with its name, its
	// tooltip (hidden while the menu, which says it too, is open). The
	// tooltip describes the avatar by itself, so the line is set only where
	// there is none (a second Description would be read twice).
	if !f.menu.IsOpen() {
		tip := i18n.T("Account")
		if line != "" {
			tip += "\n" + line
		}
		kit.Tooltip(c, avatar, tip)
	} else {
		avatar.Description(line)
	}
	f.menu.Show(c, kit.MenuOpts{ID: "auth:menu", Label: i18n.T("Account"), Width: 288}, avatar, f.menuItems(status, known, line, priv))
}

// avatar is the round avatar button: the active tone when signed in, muted
// otherwise, a red dot at its top end while the account needs attention.
func (f *feature) avatar(c *ui.Context, signedIn, attention, open bool) ui.Element {
	tk := theme.Current()
	bg, fg := theme.C("hover"), theme.C("ink-muted")
	if signedIn {
		bg, fg = theme.C("active"), theme.C("ink")
	}
	btn := ui.ButtonBase(c.Key("auth:avatar")).Label(i18n.T("Account")).Expanded(open).
		Size(avatarSize, avatarSize).Radius(tk.Radius["full"]).Background(bg).TextColor(fg).Center()
	if !theme.ReducedMotion() {
		m := tk.Motion["feedback"]
		btn.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
	}
	btn.Children(func() {
		ui.Icon(c.Key("auth:avatar-icon"), theme.Icon("user")).FontSize(16).TextColor(fg)
		if attention {
			ui.Box(c.Key("auth:attention")).Absolute().Top(-2).Right(-2).Size(10, 10).Radius(tk.Radius["full"]).
				Background(theme.C("danger")).Border(2, theme.C("surface"))
		}
	})
	return btn
}

// menuItems are the rows of the menu: the header (Apple Music, where the
// account stands and what is worth knowing), the account's actions, private
// listening and Sign out.
func (f *feature) menuItems(s services.AuthStatus, known bool, line string, priv store.Result[services.PrivateState]) []kit.MenuItem {
	signedIn := account.HasAccount(s.State)
	var items []kit.MenuItem
	note := func(text string) { items = append(items, kit.MenuItem{Kind: kit.MenuLabel, Label: text}) }
	sep := func() { items = append(items, kit.MenuItem{Kind: kit.MenuSeparator}) }

	note(i18n.T("Apple Music"))
	if line != "" {
		note(line)
	}
	if signedIn && s.Offline {
		note(i18n.T("Offline. Sona keeps using your saved sign-in."))
	}
	if signedIn && s.Restricted {
		note(i18n.T("Explicit content is restricted on this account; Sona hides it."))
	}
	if s.Unencrypted {
		if signedIn {
			note(i18n.T("Your token is stored unencrypted"))
		} else {
			note(i18n.T("Signing in now stores your token unencrypted"))
		}
	}
	sep()

	switch {
	case signedIn:
		items = append(items, kit.MenuItem{Key: "settings", Label: i18n.T("Account settings"), Run: f.env.Surfaces.OpenAccount})
	case known && s.State != "restoring":
		items = append(items, kit.MenuItem{Key: "sign-in", Label: i18n.T("Sign in"), Run: f.env.Surfaces.OpenAccount})
	}
	if signedIn && s.Offline {
		items = append(items, kit.MenuItem{Key: "check", Label: i18n.T("Check now"), Run: f.checkNow})
	}
	if known && !signedIn && s.State != "restoring" && (s.Keyring == "unavailable" || s.Unencrypted) {
		items = append(items, kit.MenuItem{Key: "retry", Label: i18n.T("Try again"), Run: f.retryKeyring})
	}
	sep()

	items = append(items, f.privateItems(priv)...)

	if signedIn {
		sep()
		items = append(items, kit.MenuItem{Key: "sign-out", Label: i18n.T("Sign out"), Destructive: true, Run: f.section.ConfirmSignOut})
	}
	return items
}

// privateItems is the Private listening checkbox and the line under it. It
// is off until Go has said what the state is, and while SONA_PRIVATE=1 keeps
// it on (the line is then the row's reason, which assistive technology reads
// as its description).
func (f *feature) privateItems(priv store.Result[services.PrivateState]) []kit.MenuItem {
	st := priv.Data
	forced := priv.HasData && store.PrivateForced(st)
	row := kit.MenuItem{
		Key: "private", Kind: kit.MenuCheckbox, Label: i18n.T("Private listening"),
		Checked: priv.HasData && st.On, Disabled: !priv.HasData,
		KeepOpen: true,
		Run:      func() { f.setPrivate(!st.On) },
	}
	if forced {
		row.Reason = i18n.T("On because Sona was started with SONA_PRIVATE=1")
		return []kit.MenuItem{row}
	}
	text := i18n.T("Plays stay out of your listening history, scrobbles and Discord.")
	if priv.HasData && st.Source == "development" && st.On {
		text = i18n.T("Private listening is on for this development build. Turning it off lasts until Sona quits.")
	}
	return []kit.MenuItem{row, {Kind: kit.MenuLabel, Label: text}}
}

// setPrivate switches private listening; the row keeps the menu open, so
// the tick (which follows Go's answer) is the switch's only confirmation.
func (f *feature) setPrivate(on bool) {
	kit.SetPrivate(f.env.Queries, f.env.Core, on)
}

// checkNow asks Apple to confirm the stored sign-in now; a failure toasts.
func (f *feature) checkNow() {
	api := f.d.API
	go func() {
		if err := api.Revalidate(context.Background()); err != nil {
			kit.ToastErrorf(i18n.T("Couldn't reach Apple"), i18n.ErrorMessage(err))
		}
	}()
}

// retryKeyring reads the stored sign-in from the keyring again; the outcome
// arrives as auth:changed. The read may wait for the keyring, so it is off
// the main thread.
func (f *feature) retryKeyring() {
	go f.d.API.RetryKeyring()
}
