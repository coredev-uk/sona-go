package account

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// withLink gets a new sign-in link and hands it to act on the main thread.
// Every sign-in link works once, so each button asks Sona for a new one; a
// link is never kept, logged or shown.
func (s *Section) withLink(act func(url string)) {
	api := s.d.API
	go func() {
		url, err := api.SignInLink()
		s.d.Post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't get a sign-in link"), i18n.ErrorMessage(err))
				return
			}
			act(url)
		})
	}()
}

// browserWaiting is BrowserWaiting: the browser fallback while Apple's page
// is open in the system browser.
func (s *Section) browserWaiting(c *ui.Context, st services.AuthStatus) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c.Key("head")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(20, 20).TextColor(theme.C("ink-muted")).Role(ui.RoleNone) // the title beside it names the wait
			ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
				flowTitle(c, i18n.T("Finish signing in in your browser"))
				text(c, i18n.T("Apple's sign-in page is open in your browser. Sona continues by itself when you're done."), "body", "ink-muted").Margin(2, 0, 0, 0)
			})
		})
		ui.Row(c.Key("actions")).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			if st.BrowserSignIn {
				if kit.Button(c.Key("again"), i18n.T("Open the page again"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "external-link"}).Clicked() {
					s.withLink(s.open)
				}
				if kit.Button(c.Key("copy"), i18n.T("Copy link"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
					s.withLink(func(url string) {
						if s.d.Copy != nil && s.d.Copy(url) {
							kit.ShowToast(kit.ToastSpec{Title: i18n.T("Sign-in link copied")})
							return
						}
						kit.ToastErrorf(i18n.ErrorText(services.CodeClipboard), "")
					})
				}
			}
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
				api := s.d.API
				go api.CancelSignIn()
			}
		})
	})
}
