package account

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// languageName is "en-GB" as the interface's language names it ("British
// English"); the tag when it can't.
func languageName(tag, locale string) string {
	t, err := language.Parse(tag)
	if err != nil {
		return tag
	}
	l, err := language.Parse(locale)
	if err != nil {
		l = language.English
	}
	if n := display.Languages(l).Name(t); n != "" {
		return n
	}
	return tag
}

// revalidate checks the stored token with Apple now.
func (s *Section) revalidate() {
	if s.checking {
		return
	}
	s.checking = true
	api := s.d.API
	s.call(func() error { return api.Revalidate(context.Background()) }, func(err error) {
		s.checking = false
		if err != nil {
			kit.ToastErrorf(i18n.T("Couldn't reach Apple"), i18n.ErrorMessage(err))
		}
	})
}

// signedIn is SignedIn: the account, where its token is kept, and Sign out.
func (s *Section) signedIn(c *ui.Context, st services.AuthStatus) {
	storefront := st.StorefrontName
	if storefront == "" {
		storefront = strings.ToUpper(st.StorefrontID)
	}
	ui.Column(c.Key("signed-in")).FillWidth().Gap(16).Children(func() {
		if st.Offline {
			notice(c.Key("offline"), noticeOpts{Icon: "wifi-off", IconColor: "ink-muted"}, func() {
				text(c, i18n.T("Sona couldn't reach Apple to confirm your sign-in. It keeps using your saved token, and retries by itself."), "secondary", "ink")
			}, func() {
				label := i18n.T("Check now")
				if s.checking {
					label = i18n.T("Checking…")
				}
				if kit.Button(c.Key("check"), label, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Disabled: s.checking}).Clicked() {
					s.revalidate()
				}
			})
		}
		ui.Column(c.Key("rows")).FillWidth().Children(func() {
			hairline(c.Key("top"))
			ui.Row(c.Key("who")).FillWidth().Gap(24).AlignItems(ui.Center).Padding(16, 0).Children(func() {
				ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
					switch {
					case st.Keyring == "waiting":
						text(c, i18n.T("Saving your sign-in in your keyring; unlock it if it asks."), "label", "ink").Role(ui.RoleStatus)
					case st.State == "validating":
						text(c, i18n.T("Checking your sign-in…"), "label", "ink").Role(ui.RoleStatus)
					default:
						text(c, i18n.T("Signed in to Apple Music"), "label", "ink")
					}
					sub := i18n.T("Storefront unknown until Apple confirms the token")
					if storefront != "" {
						sub = i18n.T("Storefront: {store}", "store", storefront)
					}
					if st.Language != "" {
						sub += " · " + languageName(st.Language, i18n.Locale())
					}
					text(c.Key("sub"), sub, "secondary", "ink-muted").Margin(2, 0, 0, 0)
				})
				if kit.Button(c.Key("sign-out"), i18n.T("Sign out"), kit.ButtonOpts{Variant: kit.Destructive, Size: kit.SizeSM}).Clicked() {
					s.ConfirmSignOut()
				}
			})
			if st.Restricted {
				hairline(c.Key("restricted-rule"))
				text(c.Key("restricted"), i18n.T("Explicit content is restricted on this account; Sona hides it."), "secondary", "ink-muted").Margin(16, 0, 16, 0)
			}
			// An unencrypted file gets its own notice below, which says where the token is.
			if !st.Unencrypted {
				hairline(c.Key("kept-rule"))
				ui.Column(c.Key("kept")).FillWidth().Padding(16, 0).Children(func() {
					text(c, i18n.T("Where your token is kept"), "label", "ink")
					backend := st.Backend
					if backend == "" {
						backend = i18n.T("Unknown")
					}
					text(c.Key("backend"), backend, "secondary", "ink-muted").Margin(2, 0, 0, 0)
				})
			}
			hairline(c.Key("bottom"))
		})
		if st.Unencrypted {
			notice(c.Key("unencrypted"), noticeOpts{Icon: "shield-alert", IconColor: "danger", Problem: true}, func() {
				noticeTitle(c, i18n.T("Your token is stored unencrypted"))
				noticeDetail(c, i18n.T("No Secret Service keyring is running, so Sona keeps your Apple Music token in a file only your user can read. Start GNOME Keyring or KWallet and Sona moves it into the keyring."))
			}, nil)
		}
	})
}

// ConfirmSignOut asks before signing out of Apple Music (the account
// dialog's Sign out button and the account menu's item), then signs out. A
// failure asks again, with a toast saying why. The question is kit.Confirm's
// (the shell mounts its host).
func (s *Section) ConfirmSignOut() {
	if s.signingOut {
		return
	}
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Sign out of Apple Music?"),
		Body:         i18n.T("Sona removes your token from the keyring, stops playback and deletes the copy of your library on this computer. Your Apple Music library itself isn't changed."),
		ConfirmLabel: i18n.T("Sign out"),
		BusyLabel:    i18n.T("Signing out…"),
		Destructive:  true,
		Hold:         true,
	}, func(ok bool) {
		if ok && !s.signingOut {
			s.signOut()
		}
	})
}

// signOut signs out; while it runs the held question shows "Signing out…".
func (s *Section) signOut() {
	s.signingOut = true
	api := s.d.API
	s.call(func() error { return api.SignOut(context.Background()) }, func(err error) {
		s.signingOut = false
		if err == nil {
			kit.ConfirmClose()
			return
		}
		kit.ToastErrorf(i18n.T("Couldn't sign out"), i18n.ErrorMessage(err))
		kit.ConfirmRelease() // the question asks again
	})
}
