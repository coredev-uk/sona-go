package auth

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

const (
	// dialogWidth is sm:max-w-xl (36 rem).
	dialogWidth = 576
	// dialogChrome is what the panel holds besides the body, taken from the
	// room the body may scroll in: the window's 2 rem margins (32), the
	// panel's 24 padding top and bottom, the 18 high title and the 20 gap
	// below it.
	dialogChrome = 32 + 2*24 + 18 + 20
	// minBody keeps a body in sight in a very short window.
	minBody = 120
)

// dialog is AccountDialog: the account section in a dialog, open while
// Surfaces says so. Signing in or the account's settings (account.Section),
// opened from the avatar's menu, the palette and every "Sign in" action. It
// stays open after a sign-in, which then shows the account it reached. The
// body scrolls inside the window, as max-h-[calc(100%-2rem)] overflow-y-auto
// does; when it opens, the sign-in form's Apple Account field takes the focus.
func (f *feature) dialog(c *ui.Context) {
	open := f.env.Surfaces.AccountOpen()
	if open && !f.shown {
		f.section.FocusAccount()
	}
	f.shown = open
	_, h := c.Size()
	kit.Dialog(c, &open, kit.DialogOpts{Title: i18n.T("Account"), MaxWidth: dialogWidth, Gap: 20}, func() {
		ui.Scroll(c.Key("auth:body")).FillWidth().MaxHeight(max(h-dialogChrome, minBody)).Children(func() {
			ui.Column(c.Key("auth:section")).FillWidth().Children(func() {
				f.section.Build(c, false)
			})
		})
	})
	if !open && f.shown {
		f.shown = false
		f.env.Surfaces.CloseAccount()
	}
}
