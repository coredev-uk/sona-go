package settings

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The pieces of Connections' scrobbling that are reusable (scrobble.ts and
// the Last.fm waiting panel and ListenBrainz form of Connections.tsx): plain
// methods with narrow inputs. The first-run setup keeps its own copy.

// provider is a scrobbler's service ID (ScrobblerLastFM,
// ScrobblerListenBrainz).
type provider = string

// providerName is a provider's name: brand names, never translated.
func providerName(p provider) string {
	if p == services.ScrobblerListenBrainz {
		return "ListenBrainz"
	}
	return "Last.fm"
}

// listenBrainzTokenPage is where a ListenBrainz user token is copied from.
const listenBrainzTokenPage = "https://listenbrainz.org/settings/"

// scrobbleUI is the state of the scrobbling rows that outlives a frame.
type scrobbleUI struct {
	lastfmBusy bool     // ConnectLastFM is in flight
	removing   provider // the provider being disconnected
	lb         bool     // the ListenBrainz form is open
	lbForm     tokenForm
	focusBack  bool // put the focus back on the ListenBrainz row's button
}

// tokenForm is the ListenBrainz form's state: the text, a check in flight
// and what the last attempt said.
type tokenForm struct {
	token string
	busy  bool
	err   string
}

// errorLine is what went wrong connecting a scrobbler, in a sentence (scrobble.ts
// errorLine); "" when the user cancelled.
func errorLine(p provider, err error) string {
	switch services.CodeOf(err) {
	case services.CodeCancelled:
		return ""
	case services.CodeInvalidCredentials:
		if p == services.ScrobblerListenBrainz {
			return i18n.T("ListenBrainz didn't accept that token. Copy it again from your ListenBrainz settings.")
		}
		return i18n.T("Last.fm refused Sona's request. Try connecting again.")
	case services.CodeTimeout:
		return i18n.T("The Last.fm approval expired before it was confirmed. Connect again to get a new one.")
	case services.CodeNetwork:
		return i18n.T("Couldn't reach {name}. Check your connection and try again.", "name", providerName(p))
	case services.CodeNotConfigured:
		return i18n.T("This build of Sona has no Last.fm API key, so it can't connect to Last.fm.")
	}
	return i18n.ErrorMessage(err)
}

// pendingLine is how many songs a disconnect would discard.
func pendingLine(n int) string {
	return i18n.T("{n, plural, one {# song you played hasn't been sent yet.} other {# songs you played haven't been sent yet.}}", "n", n)
}

// connectLastFM starts Last.fm's desktop authorisation (the browser opens, Go
// waits for the approval) and stores the status it ends with. A failure other
// than a cancel is a toast (useConnectLastFM). busy covers the call; Go's
// connecting the wait.
func (s *settings) connectLastFM(busy *bool) {
	calls := s.scrobbler
	if *busy {
		return
	}
	*busy = true
	go func() {
		next, err := calls.ConnectLastFM(context.Background())
		s.env.Post(func() {
			*busy = false
			if err != nil {
				if line := errorLine(services.ScrobblerLastFM, err); line != "" {
					kit.ToastErrorf(i18n.T("Couldn't connect Last.fm"), line)
				}
				return
			}
			store.Set(s.env.Queries, keyScrobble, next)
		})
	}()
}

// cancelLastFM abandons a pending Last.fm approval.
func (s *settings) cancelLastFM() {
	go s.scrobbler.CancelConnect()
}

// submitListenBrainz checks the token of f with ListenBrainz and stores it:
// done gets the new status, a failure leaves its sentence in f.err.
func (s *settings) submitListenBrainz(f *tokenForm, done func(services.ScrobbleStatus)) {
	calls := s.scrobbler
	if f.busy {
		return
	}
	f.busy, f.err = true, ""
	token := f.token
	go func() {
		next, err := calls.ConnectListenBrainz(context.Background(), token)
		s.env.Post(func() {
			f.busy = false
			if err != nil {
				f.err = errorLine(services.ScrobblerListenBrainz, err)
				return
			}
			done(next)
		})
	}()
}

// disconnect removes a provider's sign-in and queued songs; busy names the
// provider while it runs.
func (s *settings) disconnect(busy *provider, p provider) {
	calls := s.scrobbler
	if *busy != "" {
		return
	}
	*busy = p
	go func() {
		next, err := calls.Disconnect(p)
		s.env.Post(func() {
			*busy = ""
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't disconnect {name}", "name", providerName(p)), i18n.ErrorMessage(err))
				kit.ConfirmRelease() // a held "Disconnecting…" question asks again
				return
			}
			kit.ConfirmClose()
			store.Set(s.env.Queries, keyScrobble, next)
		})
	}()
}

// lastFMWaiting is the status panel of a Last.fm authorisation under way: a
// spinner, what to do in the browser and Cancel.
func (s *settings) lastFMWaiting(c *ui.Context, cancel func()) {
	c = c.Key("lastfm-waiting")
	msg := i18n.T("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself.")
	row := ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Padding(12, 16).Radius(theme.Current().Radius["md"]).
		Background(theme.C("hover")).Role(ui.RoleStatus).Label(msg)
	kit.AnnounceMounted(c, row, msg)
	row.
		Children(func() {
			ic := ui.Icon(c.Key("spin"), theme.Icon("loader-circle")).FontSize(16).TextColor(theme.C("ink-muted")).Shrink(0)
			theme.Spin(ic, true)
			theme.Text(c, i18n.T("Allow Sona on the Last.fm page that opened in your browser. Sona continues by itself."), "secondary").
				TextColor(theme.C("ink")).Grow(1).Shrink(1).MinWidth(0)
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
				cancel()
			}
		})
}

// listenBrainzForm is the user token form under the ListenBrainz row: a
// password field, Connect (disabled while the token is blank or being
// checked) and Cancel, Escape cancelling, a hint with a link to the token
// page and an alert when the token was refused. f is the form's state;
// connect runs the check (submitListenBrainz); cancel closes the form.
func (s *settings) listenBrainzForm(c *ui.Context, f *tokenForm, connect func(), cancel func()) {
	c = c.Key("lb-form")
	form := ui.Column(c).FillWidth().Gap(8)
	form.Children(func() {
		theme.Text(c, i18n.T("User token"), "label").TextColor(theme.C("ink"))
		ui.Row(c.Key("fields")).FillWidth().Gap(8).Children(func() {
			in := kit.Input(c.Key("token"), &f.token, kit.InputOpts{A11yLabel: i18n.T("User token"), Password: true, Invalid: f.err != "", Error: f.err})
			in.Grow(1).Shrink(1).MinWidth(0).Height(32).AutoFocus()
			ready := !f.busy && trim(f.token) != ""
			label := i18n.T("Connect")
			if f.busy {
				label = i18n.T("Checking…")
			}
			submit := kit.Button(c.Key("connect"), label, kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Disabled: !ready}).Clicked()
			if (in.Submitted() || submit) && ready {
				connect()
			}
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
				cancel()
			}
		})
		s.tokenHint(c)
		if f.err != "" {
			s.alertLine(c.Key("err"), "circle-alert", f.err)
		}
	})
	if form.Shortcut(0, ui.KeyEscape) {
		cancel()
	}
}

// tokenHint is "Copy it from your ListenBrainz settings ↗. Sona checks it …":
// the translation marks the link as <0>text<1/></0>.
func (s *settings) tokenHint(c *ui.Context) {
	msg := i18n.T("Copy it from <0>your ListenBrainz settings<1/></0>. Sona checks it with ListenBrainz and keeps it in your keyring.")
	before, rest, _ := strings.Cut(msg, "<0>")
	link, after, ok := strings.Cut(rest, "<1/></0>")
	c = c.Key("hint")
	if !ok {
		theme.Text(c, msg, "meta").TextColor(theme.C("ink-muted"))
		return
	}
	ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Children(func() {
		theme.Text(c.Key("a"), before, "meta").TextColor(theme.C("ink-muted"))
		if kit.Button(c.Key("link"), link, kit.ButtonOpts{Variant: kit.Link, Size: kit.SizeXS, Icon: "external-link"}).Clicked() {
			_ = s.env.OpenURL(listenBrainzTokenPage)
		}
		theme.Text(c.Key("b"), after, "meta").TextColor(theme.C("ink-muted"))
	})
}

// alertLine is a sentence with a danger icon, an alert read as it appears.
func (s *settings) alertLine(c *ui.Context, icon, msg string) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Role(ui.RoleAlert).Label(msg).Children(func() {
		ui.Icon(c.Key("i"), theme.Icon(icon)).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
		theme.Text(c, msg, "secondary").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
	})
}
