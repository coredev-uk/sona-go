package account

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// minToken is the shortest text the form takes for a Music User Token.
const minToken = 20

// tokenErrorText is the wording of a failed token check: Apple's refusal and
// the network have their own; anything else is the failure's code wording.
func tokenErrorText(err error) string {
	switch code := services.CodeOf(err); code {
	case services.CodeMUTRejected, services.CodeUnauthorized:
		return i18n.T("Apple didn't accept that token. Copy a fresh one from a signed-in Apple Music web session.")
	case services.CodeNetwork:
		return i18n.T("Couldn't reach Apple to check the token. Check your connection and try again.")
	default:
		return i18n.ErrorText(code)
	}
}

// tokenPaste is TokenPaste: the manual token paste, the last-resort
// fallback for developers and odd accounts.
func (s *Section) tokenPaste(c *ui.Context, f *flow) {
	if !f.tokenOpen {
		// A bare text button: muted ink, flush left, ink and underlined under
		// the pointer (text-ink-muted hover:text-ink hover:underline).
		label := i18n.T("Paste a Music User Token")
		b := ui.ButtonBase(c.Key("open")).Label(label).Gap(8).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		fg := theme.C("ink-muted")
		if b.Hovered() {
			fg = theme.C("ink")
		}
		hovered := b.Hovered()
		b.Children(func() {
			ui.Icon(c, theme.Icon("key-round")).FontSize(16).TextColor(fg)
			t := text(c, label, "body", "ink-muted").TextColor(fg).SingleLine()
			if hovered {
				t.Underline()
			}
		})
		if b.Clicked() {
			f.tokenOpen = true
		}
		return
	}
	submitted := false
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		field(c.Key("field"), i18n.T("Music User Token"), i18n.T("The token from a signed-in Apple Music web session. Sona checks it with Apple and keeps it in your keyring."), func() {
			t := theme.Current().Type["secondary"]
			in := ui.TextAreaBase(c.Key("mut"), &f.token)
			in.Lines(3, 3).Padding(8, 12).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(theme.C("ink")).FillWidth()
			fieldFace(in)
		})
		if f.tokenError != "" {
			row := ui.Row(c.Key("error")).FillWidth().AlignItems(ui.Start).Gap(8).Role(ui.RoleStatus)
			if last := ui.Local(row, "announced", func() string { return "" }); *last != f.tokenError {
				*last = f.tokenError
				c.Announce(f.tokenError)
			}
			row.Children(func() {
				ui.Icon(c, theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
				text(c, f.tokenError, "secondary", "ink").Grow(1).MinWidth(0)
			})
		}
		ui.Row(c.Key("buttons")).Gap(8).Children(func() {
			label := i18n.T("Use this token")
			if f.tokenBusy {
				label = i18n.T("Checking…")
			}
			if kit.Button(c.Key("use"), label, kit.ButtonOpts{
				Variant: kit.Outline, Size: kit.SizeSM, Disabled: f.tokenBusy || len(trimmed(f.token)) < minToken,
			}).Clicked() {
				submitted = true
			}
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}).Clicked() {
				f.tokenOpen, f.token, f.tokenError = false, "", ""
			}
		})
	})
	if submitted {
		s.submitToken(f)
	}
}

// submitToken checks the trimmed token with Apple; it is cleared on success.
func (s *Section) submitToken(f *flow) {
	token := trimmed(f.token)
	if f.tokenBusy || len(token) < minToken {
		return
	}
	f.tokenBusy, f.tokenError = true, ""
	api := s.d.API
	s.call(func() error { return api.SignInWithToken(context.Background(), token) }, func(err error) {
		if s.flow != f {
			return
		}
		f.tokenBusy = false
		if err != nil {
			f.tokenError = tokenErrorText(err)
			return
		}
		f.token = ""
	})
}
