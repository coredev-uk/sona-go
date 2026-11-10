package account

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// codeLength is the code's length (default 6).
func codeLength(f *services.SecondFactor) int {
	if f.CodeLength > 0 {
		return f.CodeLength
	}
	return 6
}

// digits keeps the digits of v, at most n of them.
func digits(v string, n int) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' && b.Len() < n {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func phoneLabel(f *services.SecondFactor, phoneID int) string {
	for _, p := range f.Phones {
		if p.ID == phoneID {
			return p.Number
		}
	}
	return i18n.T("your phone")
}

// sendCode asks Apple to text or call phoneID and moves the form to its code.
func (s *Section) sendCode(f *flow, phoneID int, mode DeliveryMode) {
	f.apply(SignInAction{Type: ActSendToPhone, PhoneID: phoneID, Mode: mode})
	api := s.d.API
	s.call(func() error { return api.SendCodeToPhone(context.Background(), phoneID, string(mode)) }, func(err error) {
		if s.flow != f {
			return
		}
		if err != nil {
			f.apply(SignInAction{Type: ActFailed, Error: classifySignInError(err, "", "")})
			return
		}
		f.apply(SignInAction{Type: ActSent, Target: CodeTarget{PhoneID: phoneID, Mode: mode}})
	})
}

// submitCode verifies a full-length code; a wrong one clears the field and
// shows the error, a right one moves to done.
func (s *Section) submitCode(f *flow, value string) {
	step := f.step
	if step.Kind != StepCode || step.Busy || len(value) != codeLength(step.Factor) {
		return
	}
	target := step.Target
	f.apply(SignInAction{Type: ActSubmit})
	api := s.d.API
	s.call(func() error {
		return api.SubmitSecurityCode(context.Background(), value, target.PhoneID, string(target.Mode))
	}, func(err error) {
		if s.flow != f {
			return
		}
		if err != nil {
			f.code = ""
			f.apply(SignInAction{Type: ActFailed, Error: classifySignInError(err, "", "")})
			return
		}
		f.apply(SignInAction{Type: ActSignedIn})
	})
}

// resend sends a new code the way the current one was sent.
func (s *Section) resend(f *flow) {
	target := f.step.Target
	f.apply(SignInAction{Type: ActResend})
	api := s.d.API
	s.call(func() error {
		if target.PhoneID == 0 {
			return api.ResendSecurityCode(context.Background())
		}
		return api.SendCodeToPhone(context.Background(), target.PhoneID, string(target.Mode))
	}, func(err error) {
		if s.flow != f {
			return
		}
		if err != nil {
			f.apply(SignInAction{Type: ActFailed, Error: classifySignInError(err, "", "")})
			return
		}
		f.apply(SignInAction{Type: ActResent})
	})
}

// cancelSignIn abandons the attempt Go holds and returns to an empty form.
func (s *Section) cancelSignIn(f *flow) {
	api := s.d.API
	go api.CancelSignIn()
	f.apply(SignInAction{Type: ActCancel})
}

// pickMethod is the method switcher's choice in the code step: the device
// code, a text or call to the only number now, or the choice of number.
func (s *Section) pickMethod(f *flow, m CodeMethod) {
	phones := f.step.Factor.Phones
	switch {
	case m == MethodDevice:
		f.apply(SignInAction{Type: ActUseDevice})
	case len(phones) == 1:
		s.sendCode(f, phones[0].ID, DeliveryMode(m))
	default:
		f.apply(SignInAction{Type: ActChoosePhone, Mode: DeliveryMode(m)})
	}
}

// codeForm is CodeForm: where the code went, the method switcher, the code
// field (submitting at its last digit) and the resend / cancel buttons.
func (s *Section) codeForm(c *ui.Context, f *flow) {
	step := f.step
	n := codeLength(step.Factor)
	phoneID, mode := step.Target.PhoneID, step.Target.Mode
	phones := step.Factor.Phones

	phone := phoneLabel(step.Factor, phoneID)
	var where string
	switch {
	case phoneID == 0:
		where = i18n.T("Apple sent a {len}-digit code to the devices signed in to this Apple Account. Enter it here to finish signing in.", "len", n)
	case mode == ModeVoice:
		where = i18n.T("Apple is calling {phone} with a {len}-digit code.", "phone", phone, "len", n)
	default:
		where = i18n.T("Apple sent a {len}-digit code to {phone} by text message.", "phone", phone, "len", n)
	}

	var picked CodeMethod
	submitted := false
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Column(c.Key("head")).FillWidth().Children(func() {
			flowTitle(c, i18n.T("Enter the verification code"))
			text(c, where, "body", "ink-muted").Margin(4, 0, 0, 0)
		})
		picked = s.methodSwitcher(c.Key("methods"), step.Factor, currentMethod(step), step.Busy)
		if step.Error != nil {
			s.errorNotice(c.Key("error"), *step.Error, nil)
		}
		if step.Notice != "" && step.Error == nil {
			n := text(c.Key("notice"), step.Notice, "secondary", "ink").Role(ui.RoleStatus)
			if last := ui.Local(n, "announced", func() string { return "" }); *last != step.Notice {
				*last = step.Notice
				c.Announce(step.Notice)
			}
		}
		field(c.Key("code-field"), i18n.T("Code"), i18n.T("{len, plural, one {# digit} other {# digits}}", "len", n), func() {
			t := theme.Current().Type["heading"]
			wrong := step.Error != nil && step.Error.Code == services.CodeIncorrectCode
			wrongText := ""
			if wrong {
				wrongText = errorCopy(*step.Error, s.d.OS).Title
			}
			in := kit.Input(c.Key("code"), &f.code, kit.InputOpts{Invalid: wrong, Error: wrongText, Disabled: step.Busy})
			kit.FocusWhenShown(c, in, &f.codeFocus)
			in.TextAlign(ui.Center).FontSize(t.Size).FixedLineHeight(t.LineHeight).
				LetterSpacing(0.4 * t.Size).FontFeatures("tnum").Width((float32(n)*1.6 + 2) * t.Size).Shrink(0)
			if in.Changed() {
				v := digits(f.code, n)
				f.code = v
				if len(v) == n {
					s.submitCode(f, v)
				}
			}
			if in.Submitted() {
				submitted = true
			}
		})
		ui.Row(c.Key("buttons")).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			label, icon := i18n.T("Verify"), ""
			if step.Busy {
				label, icon = i18n.T("Checking…"), "loader-circle"
			}
			if kit.Button(c.Key("verify"), label, kit.ButtonOpts{Icon: icon, Spin: step.Busy, Disabled: step.Busy || len(f.code) != n}).Clicked() {
				submitted = true
			}
			again := i18n.T("Send a new code")
			if phoneID != 0 {
				again = i18n.T("Text again")
				if mode == ModeVoice {
					again = i18n.T("Call again")
				}
			}
			if kit.Button(c.Key("resend"), again, kit.ButtonOpts{Variant: kit.Ghost, Disabled: step.Busy}).Clicked() {
				s.resend(f)
			}
			if phoneID != 0 && len(phones) > 1 {
				if kit.Button(c.Key("another"), i18n.T("Use another number"), kit.ButtonOpts{Variant: kit.Ghost, Disabled: step.Busy}).Clicked() {
					f.apply(SignInAction{Type: ActChoosePhone})
				}
			}
			ui.Box(c.Key("spacer")).Grow(1)
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
				s.cancelSignIn(f)
			}
		})
	})
	if picked != "" {
		s.pickMethod(f, picked)
	}
	if submitted {
		s.submitCode(f, f.code)
	}
}

// phoneChoice is PhoneChoice: which trusted number Apple texts or calls.
func (s *Section) phoneChoice(c *ui.Context, f *flow) {
	step := f.step
	phones := step.Factor.Phones
	mode := step.Mode
	var picked CodeMethod
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Column(c.Key("head")).FillWidth().Children(func() {
			title := i18n.T("Which number should Apple text?")
			if mode == ModeVoice {
				title = i18n.T("Which number should Apple call?")
			}
			flowTitle(c, title)
			text(c, i18n.T("Choose one of the trusted phone numbers on this Apple Account."), "body", "ink-muted").Margin(4, 0, 0, 0)
		})
		picked = s.methodSwitcher(c.Key("methods"), step.Factor, CodeMethod(mode), step.Busy)
		if step.Error != nil {
			s.errorNotice(c.Key("error"), *step.Error, nil)
		}
		ui.RadioGroup(c.Key("numbers"), func() {
			for _, p := range phones {
				on := f.phoneID == p.ID
				r := ui.Radio(c.Key(p.ID), &f.phoneID, p.ID, p.Number)
				r.Height(40).PaddingX(12).Radius(theme.Current().Radius["md"]).TextColor(theme.C("ink"))
				switch {
				case on:
					r.Background(theme.C("active"))
				case r.Hovered():
					r.Background(theme.C("hover"))
				}
			}
		}).Label(i18n.T("Trusted phone number"))
		ui.Row(c.Key("buttons")).Gap(8).AlignItems(ui.Center).Children(func() {
			label, icon := i18n.T("Text me"), ""
			if mode == ModeVoice {
				label = i18n.T("Call me")
			}
			if step.Busy {
				label, icon = i18n.T("Sending…"), "loader-circle"
			}
			if kit.Button(c.Key("send"), label, kit.ButtonOpts{Icon: icon, Spin: step.Busy, Disabled: step.Busy || f.phoneID == 0}).Clicked() {
				s.sendCode(f, f.phoneID, mode)
			}
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Ghost}).Clicked() {
				s.cancelSignIn(f)
			}
		})
	})
	switch picked {
	case "":
	case MethodDevice:
		f.apply(SignInAction{Type: ActUseDevice})
	default:
		f.apply(SignInAction{Type: ActChoosePhone, Mode: DeliveryMode(picked)})
	}
}

// methodSwitcher is every way to get the code, always shown: the Apple
// device option is disabled for accounts without a trusted device, text and
// call for accounts without a trusted number, all of them while busy. It
// returns the method the user picked this frame, "" for none.
func (s *Section) methodSwitcher(c *ui.Context, f *services.SecondFactor, current CodeMethod, busy bool) CodeMethod {
	options := []struct {
		method      CodeMethod
		label, icon string
	}{
		{MethodDevice, i18n.T("Apple device"), "monitor-smartphone"},
		{MethodSMS, i18n.T("Text message"), "message-square-text"},
		{MethodVoice, i18n.T("Phone call"), "phone"},
	}
	var picked CodeMethod
	ui.Column(c).AlignItems(ui.Start).Gap(6).Children(func() {
		sel := current
		ui.RadioGroup(c.Key("group"), func() {
			for _, o := range options {
				disabled := busy || (o.method == MethodDevice && !f.TrustedDevices) || (o.method != MethodDevice && len(f.Phones) == 0)
				r := ui.RadioBase(c.Key(string(o.method)), &sel, o.method)
				fg := theme.C("ink-muted")
				switch {
				case current == o.method:
					r.Background(theme.C("raised"))
					fg = theme.C("ink")
				case r.Hovered() && !disabled:
					fg = theme.C("ink")
				}
				r.Height(28).PaddingX(12).Gap(6).AlignItems(ui.Center).Radius(5).TextColor(fg).Label(o.label).Disabled(disabled)
				r.Children(func() {
					theme.Glyph(c, o.icon, 14, theme.GlyphOpts{Color: fg, Inherit: true})
					text(c, o.label, "secondary", "ink").TextColor(fg).SingleLine()
				})
				if r.Changed() {
					picked = o.method
				}
			}
		}).Row().Gap(4).Padding(4).Radius(theme.Current().Radius["md"]).Background(theme.C("hover")).Label(i18n.T("How to get the code"))
		if !f.TrustedDevices {
			text(c.Key("note"), i18n.T("No Apple device is signed in to this account, so Apple sends codes by text or call."), "meta", "ink-muted")
		}
	})
	return picked
}
