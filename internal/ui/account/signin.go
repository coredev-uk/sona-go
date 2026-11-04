package account

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// The native sign-in flow (docs/auth.md §4.7) as a pure reducer: the form
// dispatches what the user did and what Go answered; the reducer decides
// which step to show and which error, with which next step. It is v0.2.0's
// features/auth/signin.ts: no widget, no service, no state of its own.

// DeliveryMode is how a code reaches a phone.
type DeliveryMode string

const (
	ModeSMS   DeliveryMode = "sms"
	ModeVoice DeliveryMode = "voice"
)

// CodeTarget is where the code the user is typing was sent. PhoneID 0 is a
// trusted device.
type CodeTarget struct {
	PhoneID int
	Mode    DeliveryMode
}

// SignInError is a sign-in failure by its stable services error code: one of
// the codes in knownCodes, else services.CodeUnknown.
type SignInError struct {
	Code services.ErrorCode
	// Message is the code's wording (i18n.ErrorText), shown when errorCopy
	// has none of its own; for an unmapped sign-in code Apple's own
	// (already localised) message when Apple sent one.
	Message string
}

// NextStep is what the user can do next about an error.
type NextStep int

const (
	NextRetry NextStep = iota
	NextCode
	NextBrowser
	NextAccountSite
	NextRestart
)

// ErrorCopy is an error as the notice shows it.
type ErrorCopy struct {
	Title, Detail string
	Next          NextStep
}

// knownCodes are the services error codes the sign-in words itself;
// every other code is "unknown".
var knownCodes = map[services.ErrorCode]bool{
	services.CodeInvalidCredentials: true,
	services.CodeIncorrectCode:      true,
	services.CodeLocked:             true,
	services.CodeActionRequired:     true,
	services.CodeNoSubscription:     true,
	services.CodeUnsupportedCountry: true,
	services.CodeUnsupported2FA:     true,
	services.CodeOutOfOrder:         true,
	services.CodeCancelled:          true,
	services.CodeTimeout:            true,
	services.CodeNetwork:            true,
	services.CodeKeyring:            true,
}

// classifySignInError classifies a failed service call, or an
// AuthStatus.LastErrorCode (statusCode, with its LastAppleMessage) by its
// services error code, never by Go's text. Other codes are "unknown", with
// their own wording, or Apple's own (already localised) message when Apple
// sent one Sona has no code for.
func classifySignInError(err error, statusCode services.ErrorCode, statusAppleMessage string) SignInError {
	code := statusCode
	if code == "" {
		code = services.CodeOf(err)
	}
	message := i18n.ErrorText(code)
	if knownCodes[code] {
		return SignInError{Code: code, Message: message}
	}
	apple := services.AppleMessage(err)
	if statusCode != "" {
		apple = statusAppleMessage
	}
	if apple != "" {
		message = apple
	}
	return SignInError{Code: services.CodeUnknown, Message: message}
}

// errorCopy is the message and next step for each case in docs/auth.md §4.7.
func errorCopy(e SignInError) ErrorCopy {
	switch e.Code {
	case services.CodeInvalidCredentials:
		return ErrorCopy{
			Title:  i18n.T("That Apple Account or password isn't right"),
			Detail: i18n.T("Check the email or phone number and the password, then try again."),
			Next:   NextRetry,
		}
	case services.CodeIncorrectCode:
		return ErrorCopy{
			Title:  i18n.T("That code isn't right"),
			Detail: i18n.T("Enter the latest code you received. Older codes stop working once a new one is sent."),
			Next:   NextCode,
		}
	case services.CodeLocked:
		return ErrorCopy{
			Title:  i18n.T("This Apple Account is locked"),
			Detail: i18n.T("Apple has locked the account or made it temporarily unavailable. Unlock it at iforgot.apple.com, or sign in with your browser."),
			Next:   NextBrowser,
		}
	case services.CodeActionRequired:
		return ErrorCopy{
			Title:  i18n.T("Your Apple Account needs attention"),
			Detail: i18n.T("Apple wants you to review your account (new terms or a security update) before it can sign in. Do that at account.apple.com, then try again here."),
			Next:   NextAccountSite,
		}
	case services.CodeNoSubscription:
		return ErrorCopy{
			Title:  i18n.T("Apple Music subscription required"),
			Detail: i18n.T("This Apple Account doesn't have an active Apple Music subscription. Subscribe on another device, or sign in with an account that has one."),
			Next:   NextRetry,
		}
	case services.CodeUnsupportedCountry:
		return ErrorCopy{
			Title:  i18n.T("Apple Music isn't available in this account's country"),
			Detail: i18n.T("The account's country or region has no Apple Music storefront."),
			Next:   NextRetry,
		}
	case services.CodeUnsupported2FA:
		return ErrorCopy{
			Title:  i18n.T("This account needs your browser to sign in"),
			Detail: i18n.T("Its two-factor method (such as a security key) can't be completed inside Sona. Sign in with your browser instead."),
			Next:   NextBrowser,
		}
	case services.CodeOutOfOrder, services.CodeCancelled:
		return ErrorCopy{
			Title:  i18n.T("This sign-in was cancelled"),
			Detail: i18n.T("It was replaced or cancelled before the code arrived. Start again."),
			Next:   NextRestart,
		}
	case services.CodeTimeout:
		return ErrorCopy{
			Title:  i18n.T("Sign-in timed out"),
			Detail: i18n.T("No code was entered in time, so Apple closed the attempt. Start again."),
			Next:   NextRestart,
		}
	case services.CodeNetwork:
		return ErrorCopy{
			Title:  i18n.T("Couldn't reach Apple"),
			Detail: i18n.T("Check your connection and try again."),
			Next:   NextRetry,
		}
	case services.CodeKeyring:
		return ErrorCopy{
			Title:  i18n.T("Sona couldn't save your sign-in"),
			Detail: i18n.T("Your keyring didn't answer or refused, so nothing was saved. Unlock GNOME Keyring or KWallet, then sign in again."),
			Next:   NextRetry,
		}
	}
	return ErrorCopy{Title: i18n.T("Sign-in didn't work"), Detail: e.Message, Next: NextRetry}
}

// attemptStaysOpen are the errors after which Go keeps the two-factor
// attempt open (docs/auth.md §4.7).
func attemptStaysOpen(code services.ErrorCode) bool {
	switch code {
	case services.CodeIncorrectCode, services.CodeNetwork, services.CodeUnknown:
		return true
	}
	return false
}

// StepKind is which step of the sign-in the form shows.
type StepKind int

const (
	// StepCredentials is the account and password form (also where a
	// browser sign-in waits).
	StepCredentials StepKind = iota
	// StepCode is the verification code entry.
	StepCode
	// StepChoosePhone is the choice of the number to text or call.
	StepChoosePhone
	// StepDone is a sign-in Go accepted: the form waits for the status.
	StepDone
)

// SignInStep is the form's step: a value, replaced by the reducer.
type SignInStep struct {
	Kind StepKind
	Busy bool
	// Error is the failure to show (credentials, code, choose-phone).
	Error *SignInError
	// Factor is the pending challenge (code, choose-phone); never changed.
	Factor *services.SecondFactor
	// Target is where the code was sent (code).
	Target CodeTarget
	// Notice is a one-line confirmation, e.g. after a resend (code).
	Notice string
	// Mode is the method preselected: text or call (choose-phone).
	Mode DeliveryMode
}

// ActionType is what happened to the form.
type ActionType int

const (
	ActSubmit ActionType = iota
	ActFailed
	ActSecondFactor
	ActSignedIn
	// ActChoosePhone picks a number for a text or call (accounts with
	// several numbers); Mode may be "" to keep the method.
	ActChoosePhone
	// ActUseDevice switches back to the code Apple showed on the devices.
	ActUseDevice
	ActSendToPhone
	ActSent
	ActResend
	ActResent
	ActCancel
	// ActStatus is the auth status changing under the form (another
	// window, a timeout, a restart).
	ActStatus
)

// SignInAction is what the form dispatches; Type says which fields it uses.
type SignInAction struct {
	Type ActionType
	// Error: ActFailed.
	Error SignInError
	// Factor: ActSecondFactor, ActStatus (may be nil).
	Factor *services.SecondFactor
	// Mode: ActChoosePhone (optional), ActSendToPhone.
	Mode DeliveryMode
	// PhoneID: ActSendToPhone.
	PhoneID int
	// Target: ActSent.
	Target CodeTarget
	// State, LastErrorCode and LastAppleMessage: ActStatus.
	State            string
	LastErrorCode    services.ErrorCode
	LastAppleMessage string
}

// initialSignIn is an empty credentials form.
var initialSignIn = SignInStep{Kind: StepCredentials}

// CodeMethod is how the user gets the code; "device" is the Apple device
// prompt.
type CodeMethod string

const (
	MethodDevice CodeMethod = "device"
	MethodSMS    CodeMethod = "sms"
	MethodVoice  CodeMethod = "voice"
)

// currentMethod is the method a step is on, for the method switcher; "" for
// a step with none.
func currentMethod(step SignInStep) CodeMethod {
	switch step.Kind {
	case StepCode:
		if step.Target.PhoneID == 0 {
			return MethodDevice
		}
		return CodeMethod(step.Target.Mode)
	case StepChoosePhone:
		return CodeMethod(step.Mode)
	}
	return ""
}

func phoneMode(m string) DeliveryMode {
	if m == string(ModeVoice) {
		return ModeVoice
	}
	return ModeSMS
}

// stepForFactor is the step a fresh challenge opens on. Every route the
// account has is offered: with trusted devices Apple has already prompted
// them, so the device code comes first and text or call stay one click away.
// Without devices: the number Apple already texted, else the number choice.
func stepForFactor(f *services.SecondFactor) SignInStep {
	if f.TrustedDevices {
		return SignInStep{Kind: StepCode, Factor: f, Target: CodeTarget{PhoneID: 0, Mode: ModeSMS}}
	}
	if f.SentToPhone != 0 {
		mode := ModeSMS
		for _, p := range f.Phones {
			if p.ID == f.SentToPhone {
				mode = phoneMode(p.Mode)
			}
		}
		return SignInStep{Kind: StepCode, Factor: f, Target: CodeTarget{PhoneID: f.SentToPhone, Mode: mode}}
	}
	if len(f.Phones) > 0 {
		return SignInStep{Kind: StepChoosePhone, Factor: f, Mode: phoneMode(f.Phones[0].Mode)}
	}
	return SignInStep{
		Kind:  StepCredentials,
		Error: &SignInError{Code: services.CodeUnsupported2FA, Message: i18n.T("No way to receive a code.")},
	}
}

// signInReducer is the next step after action.
func signInReducer(step SignInStep, a SignInAction) SignInStep {
	switch a.Type {
	case ActSubmit, ActResend, ActSendToPhone:
		if step.Kind == StepDone {
			return step
		}
		step.Busy, step.Error = true, nil
		if step.Kind == StepCode {
			step.Notice = ""
		}
		return step

	case ActFailed:
		if (step.Kind == StepCode || step.Kind == StepChoosePhone) && attemptStaysOpen(a.Error.Code) {
			step.Busy = false
			e := a.Error
			step.Error = &e
			return step
		}
		e := a.Error
		return SignInStep{Kind: StepCredentials, Error: &e}

	case ActSecondFactor:
		return stepForFactor(a.Factor)

	case ActSignedIn:
		return SignInStep{Kind: StepDone}

	case ActChoosePhone:
		if step.Kind != StepCode && step.Kind != StepChoosePhone {
			return step
		}
		if len(step.Factor.Phones) == 0 {
			return step
		}
		mode := a.Mode
		if mode == "" {
			switch {
			case step.Kind == StepChoosePhone:
				mode = step.Mode
			case step.Target.PhoneID == 0:
				mode = ModeSMS
			default:
				mode = step.Target.Mode
			}
		}
		return SignInStep{Kind: StepChoosePhone, Factor: step.Factor, Mode: mode}

	case ActUseDevice:
		// Accounts without an Apple device have no device route.
		if (step.Kind != StepCode && step.Kind != StepChoosePhone) || !step.Factor.TrustedDevices {
			return step
		}
		return SignInStep{Kind: StepCode, Factor: step.Factor, Target: CodeTarget{PhoneID: 0, Mode: ModeSMS}}

	case ActSent:
		if step.Kind != StepCode && step.Kind != StepChoosePhone {
			return step
		}
		notice := i18n.T("Apple sent a code by text message.")
		if a.Target.Mode == ModeVoice {
			notice = i18n.T("Apple is calling with your code.")
		}
		return SignInStep{Kind: StepCode, Factor: step.Factor, Target: a.Target, Notice: notice}

	case ActResent:
		if step.Kind != StepCode {
			return step
		}
		step.Busy, step.Error = false, nil
		if step.Target.PhoneID == 0 {
			step.Notice = i18n.T("A new code is on its way to your devices.")
		} else {
			step.Notice = i18n.T("A new code is on its way.")
		}
		return step

	case ActCancel:
		return initialSignIn

	case ActStatus:
		switch {
		case a.State == "signed_in" || a.State == "validating":
			if step.Kind == StepCredentials && !step.Busy {
				return step
			}
			return SignInStep{Kind: StepDone}
		case a.State == "signing_in" && a.Factor != nil:
			// Restore a pending challenge the form doesn't know about yet.
			if step.Kind == StepCredentials && !step.Busy {
				return stepForFactor(a.Factor)
			}
			return step
		case step.Kind == StepCode || step.Kind == StepChoosePhone:
			// Go dropped the attempt (deadline, cancel, sign-out elsewhere).
			e := SignInError{Code: services.CodeCancelled}
			if a.LastErrorCode != "" {
				e = classifySignInError(nil, a.LastErrorCode, a.LastAppleMessage)
			}
			return SignInStep{Kind: StepCredentials, Error: &e}
		}
		return step
	}
	return step
}
