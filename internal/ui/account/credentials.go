package account

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// credentialsForm is CredentialsForm: the error notice, the Apple Account
// and Password fields, and the submit (left to the host when hosted).
func (s *Section) credentialsForm(c *ui.Context, f *flow, hosted bool) {
	s.credShown = true
	step := f.step
	invalid := step.Error != nil && step.Error.Code == services.CodeInvalidCredentials
	submitted := false
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		if step.Error != nil {
			s.errorNotice(c.Key("error"), *step.Error, func() { s.browser(f) })
		}
		field(c.Key("account-field"), i18n.T("Apple Account"), i18n.T("The email address or phone number you use with Apple."), func() {
			in := kit.Input(c.Key("account"), &f.account, kit.InputOpts{Invalid: invalid})
			if s.focusAccount {
				s.focusAccount = false
				in.Focus()
			}
			if in.Submitted() {
				submitted = true
			}
		})
		field(c.Key("password-field"), i18n.T("Password"), "", func() {
			if kit.Input(c.Key("password"), &f.password, kit.InputOpts{Invalid: invalid, Password: true}).Submitted() {
				submitted = true
			}
		})
		ui.Row(c.Key("submit")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			// With a host, Sign in sits in the host's foot (FootAction).
			if !hosted {
				s.signInButton(c, f, false)
			}
			text(c, i18n.T("Sona talks to Apple directly. Your password is used once and never saved."), "meta", "ink-muted").Grow(1).MinWidth(0)
		})
	})
	if submitted {
		s.submitCredentials(f)
	}
}

// signInButton is the form's submit: disabled while busy or either field is
// empty (the account trimmed); "Signing in…" with a spinner glyph while busy.
func (s *Section) signInButton(c *ui.Context, f *flow, small bool) {
	label, icon := i18n.T("Sign in"), ""
	if f.step.Busy {
		label, icon = i18n.T("Signing in…"), "loader-circle"
	}
	size := kit.SizeMD
	if small {
		size = kit.SizeSM
	}
	disabled := f.step.Busy || trimmed(f.account) == "" || f.password == ""
	if kit.Button(c.Key("sign-in"), label, kit.ButtonOpts{Size: size, Icon: icon, Disabled: disabled}).Clicked() {
		s.submitCredentials(f)
	}
}

// submitCredentials signs in with the trimmed account and the password. The
// password is used for this one exchange and never kept: the field is
// cleared at once, whatever the answer.
func (s *Section) submitCredentials(f *flow) {
	account, password := trimmed(f.account), f.password
	if f.step.Kind != StepCredentials || f.step.Busy || account == "" || password == "" {
		return
	}
	f.password = ""
	f.apply(SignInAction{Type: ActSubmit})
	api := s.d.API
	go func() {
		sf, err := api.SignInWithPassword(context.Background(), account, password)
		s.d.Post(func() {
			if s.flow != f {
				return
			}
			switch {
			case err != nil:
				f.apply(SignInAction{Type: ActFailed, Error: classifySignInError(err, "", "")})
			case sf != nil:
				f.apply(SignInAction{Type: ActSecondFactor, Factor: sf})
			default:
				f.apply(SignInAction{Type: ActSignedIn})
			}
		})
	}()
}
