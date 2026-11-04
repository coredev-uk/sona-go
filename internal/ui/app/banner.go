package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// banner is the status banner's state: the account's queries and its
// keyring retry, made on the first build (main thread).
type banner struct {
	deps  account.Deps
	retry *account.KeyringRetry
	made  bool
}

// statusBanner is v0.2.0's StatusBanner: one line under the top bar when
// the account can't reach Apple (offline, a token Apple stopped accepting, a
// rejected developer token) or waits for the keyring (the sign-in being read
// or saved, or a keyring that didn't answer), with its action. The page
// below keeps showing what SQLite has.
func (a *App) statusBanner(c *ui.Context) {
	b := &a.banner
	if !b.made {
		b.deps, b.made = account.NewDeps(a.env), true
		b.retry = account.NewKeyringRetry(b.deps)
	}
	if b.deps.API == nil {
		return
	}
	res := account.UseStatus(b.deps)
	if !res.HasData {
		return
	}
	st := res.Data
	icon, color, spin := "circle-alert", "danger", false
	var msg string
	var action func()
	switch {
	case st.Keyring == "waiting":
		icon, color, spin = "loader-circle", "ink-muted", true
		if st.State == "restoring" {
			msg = i18n.T("Signing in… Sona is waiting for your keyring; unlock it if it asks.")
			action = func() { b.retry.Build(c, st) }
		} else {
			msg = i18n.T("Saving your sign-in in your keyring; unlock it if it asks.")
		}
	case st.Keyring == "unavailable":
		msg = i18n.T("Sona couldn't read your sign-in from your keyring, so you're signed out.")
		action = func() { b.retry.Build(c, st) }
	case st.State == "needs_reauth":
		msg = i18n.T("Apple Music asked Sona to sign in again. Your library is still here.")
		action = func() {
			if kit.Button(c.Key("sign-in"), i18n.T("Sign in"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
				a.env.Surfaces.OpenAccount()
			}
		}
	case st.State == "error":
		msg = i18n.T("Apple Music isn't accepting Sona's requests.")
		action = func() {
			if kit.Button(c.Key("diagnostics"), i18n.T("Diagnostics"), kit.ButtonOpts{Variant: kit.Link, Size: kit.SizeSM}).Clicked() {
				a.router.Push("/settings?section=diagnostics")
			}
		}
	case st.Offline:
		icon, color = "wifi-off", "ink-muted"
		msg = i18n.T("Offline. Showing your library from this computer; Sona reconnects by itself.")
	default:
		return
	}
	ui.Row(c.Key("status-banner")).Height(36).FillWidth().Shrink(0).Padding(0, 32).Gap(10).AlignItems(ui.Center).
		Background(theme.C("hover")).Role(ui.RoleStatus).Children(func() {
		ic := ui.Icon(c.Key("icon"), theme.Icon(icon)).FontSize(16).TextColor(theme.C(color)).Shrink(0)
		if spin && !theme.ReducedMotion() {
			ic.Rotate(ic.Loop("spin", time.Second, ui.Linear) * 360)
		}
		text(c, msg, "sub").TextColor(theme.C("ink")).Grow(1).Shrink(1).MinWidth(0).MaxLines(1)
		if action != nil {
			action()
		}
	})
	ui.Box(c).Height(1).FillWidth().Background(theme.C("line"))
}
