package account

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// The port of signin.test.ts.

var (
	testPhones = []services.Phone{
		{ID: 1, Number: "+44 ••••• ••••12", Mode: "sms"},
		{ID: 2, Number: "+1 (•••) •••-••34", Mode: "voice"},
	}
	deviceFactor = &services.SecondFactor{CodeLength: 6, TrustedDevices: true, Phones: testPhones}
	smsOnly      = &services.SecondFactor{CodeLength: 6, Phones: testPhones, SentToPhone: 2}
	phonesOnly   = &services.SecondFactor{CodeLength: 6, Phones: testPhones}
)

func run(from SignInStep, actions ...SignInAction) SignInStep {
	for _, a := range actions {
		from = signInReducer(from, a)
	}
	return from
}

func failed(code services.ErrorCode) SignInAction {
	return SignInAction{Type: ActFailed, Error: SignInError{Code: code}}
}

func TestClassifyPrefersTheStableCodeOfTheError(t *testing.T) {
	err := fmt.Errorf("boom: %w", appleid.ErrLocked)
	got := classifySignInError(err, "", "")
	// The message is the translated wording for the code, not Go's text.
	if got.Code != services.CodeLocked || got.Message != i18n.ErrorText(services.CodeLocked) {
		t.Errorf("classified %+v", got)
	}
	if got.Message == "This Apple Account has been locked." || strings.Contains(got.Message, "boom") {
		t.Errorf("Go's text reached the message: %q", got.Message)
	}
}

func TestClassifyByCodeNeverByGoText(t *testing.T) {
	if got := classifySignInError(errors.New("auth: sign-in timed out"), "", ""); got.Code != services.CodeUnknown {
		t.Errorf("Go's text classified as %q", got.Code)
	}
	if got := classifySignInError(nil, services.CodeTimeout, ""); got.Code != services.CodeTimeout {
		t.Errorf("a status code classified as %q", got.Code)
	}
	other := classifySignInError(fmt.Errorf("auth: x: %w", auth.ErrDevTokenRejected), "", "")
	if other.Code != services.CodeUnknown || other.Message != i18n.ErrorText(services.CodeDevTokenRejected) {
		t.Errorf("an unmapped code classified as %+v", other)
	}
	// Codes the sign-in words itself, from the services.
	for err, want := range map[error]services.ErrorCode{
		appleid.ErrInvalidCredentials: services.CodeInvalidCredentials,
		appleid.ErrIncorrectCode:      services.CodeIncorrectCode,
		appleid.ErrNoSubscription:     services.CodeNoSubscription,
		auth.ErrKeyring:               services.CodeKeyring,
		appleid.ErrOutOfOrder:         services.CodeOutOfOrder,
	} {
		if got := classifySignInError(err, "", ""); got.Code != want {
			t.Errorf("%v classified as %q, want %q", err, got.Code, want)
		}
	}
}

func TestClassifyShowsApplesOwnMessageForACodeSonaDoesNotMap(t *testing.T) {
	french := "Ce compte Apple est désactivé."
	err := appleid.ParseServiceError(400, []byte(`{"serviceErrors":[{"code":"-99999","message":"`+french+`"}]}`))
	fromCall := classifySignInError(err, "", "")
	if fromCall != (SignInError{Code: services.CodeUnknown, Message: french}) {
		t.Errorf("from a call: %+v", fromCall)
	}
	if d := errorCopy(fromCall, "").Detail; d != french {
		t.Errorf("detail %q", d)
	}
	if got := classifySignInError(nil, services.CodeUnknown, french); got.Message != french {
		t.Errorf("from the status: %+v", got)
	}
	// A code Sona words keeps its own wording; Apple's text never replaces it.
	if got := classifySignInError(nil, services.CodeLocked, french); got.Message == french {
		t.Errorf("the status's Apple message replaced a mapped code: %+v", got)
	}
	if got := classifySignInError(errors.New("x"), "", ""); got.Message != "Something went wrong." {
		t.Errorf("generic message %q", got.Message)
	}
}

func TestEveryCodeHasATitleAndANextStep(t *testing.T) {
	unknown := classifySignInError(errors.New("Too many codes requested."), "", "")
	if c := errorCopy(unknown, ""); c.Detail != "Something went wrong." || c.Next != NextRetry {
		t.Errorf("unknown: %+v", c)
	}
	want := map[services.ErrorCode]NextStep{
		services.CodeInvalidCredentials: NextRetry,
		services.CodeIncorrectCode:      NextCode,
		services.CodeLocked:             NextBrowser,
		services.CodeActionRequired:     NextAccountSite,
		services.CodeNoSubscription:     NextRetry,
		services.CodeUnsupportedCountry: NextRetry,
		services.CodeUnsupported2FA:     NextBrowser,
		services.CodeOutOfOrder:         NextRestart,
		services.CodeCancelled:          NextRestart,
		services.CodeTimeout:            NextRestart,
		services.CodeNetwork:            NextRetry,
		services.CodeKeyring:            NextRetry,
	}
	for code, next := range want {
		c := errorCopy(SignInError{Code: code}, "")
		if c.Next != next || c.Title == "" || c.Detail == "" {
			t.Errorf("%s: %+v, want next %d", code, c, next)
		}
	}
	if got, again := errorCopy(SignInError{Code: services.CodeOutOfOrder}, ""), errorCopy(SignInError{Code: services.CodeCancelled}, ""); got != again {
		t.Errorf("out_of_order and cancelled differ: %+v %+v", got, again)
	}
}

// A keyring that refuses a sign-in says which keyring to unlock on this OS,
// never GNOME Keyring on a Mac or Windows.
func TestKeyringSaveErrorIsWordedPerOS(t *testing.T) {
	for goos, want := range map[string]string{
		"linux":   "Unlock GNOME Keyring or KWallet, then sign in again.",
		"":        "Unlock GNOME Keyring or KWallet, then sign in again.",
		"darwin":  "Unlock your login keychain in Keychain Access, then sign in again.",
		"windows": "Windows Credential Manager didn't answer or refused, so nothing was saved. Sign in again.",
	} {
		d := errorCopy(SignInError{Code: services.CodeKeyring}, goos).Detail
		if !strings.Contains(d, want) {
			t.Errorf("%q: detail %q, want it to contain %q", goos, d, want)
		}
		if goos == "darwin" || goos == "windows" {
			if strings.Contains(d, "GNOME") || strings.Contains(d, "KWallet") {
				t.Errorf("%q names a Linux keyring: %q", goos, d)
			}
		}
	}
}

func TestReducerOpensTheTrustedDeviceCodeStep(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSubmit}, SignInAction{Type: ActSecondFactor, Factor: deviceFactor})
	if step.Kind != StepCode || step.Target.PhoneID != 0 || step.Busy || step.Factor.CodeLength != 6 {
		t.Errorf("step %+v", step)
	}
}

func TestReducerTargetsThePhoneAppleAlreadyTexted(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: smsOnly})
	if step.Kind != StepCode || step.Target != (CodeTarget{PhoneID: 2, Mode: ModeVoice}) {
		t.Errorf("step %+v", step)
	}
}

func TestReducerAsksForAPhoneWhenNoCodeWasSentAnywhere(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: phonesOnly})
	if step.Kind != StepChoosePhone || step.Mode != ModeSMS {
		t.Fatalf("step %+v", step)
	}
	sent := run(step,
		SignInAction{Type: ActSendToPhone, PhoneID: 1, Mode: ModeSMS},
		SignInAction{Type: ActSent, Target: CodeTarget{PhoneID: 1, Mode: ModeSMS}})
	if sent.Kind != StepCode || sent.Target != (CodeTarget{PhoneID: 1, Mode: ModeSMS}) || !strings.Contains(sent.Notice, "text message") {
		t.Errorf("sent %+v", sent)
	}
}

func TestReducerSwitchesASingleNumberFromTextToACall(t *testing.T) {
	one := &services.SecondFactor{CodeLength: 6, Phones: testPhones[:1], SentToPhone: 1}
	step := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: one},
		SignInAction{Type: ActSendToPhone, PhoneID: 1, Mode: ModeVoice},
		SignInAction{Type: ActSent, Target: CodeTarget{PhoneID: 1, Mode: ModeVoice}})
	if step.Kind != StepCode || step.Target != (CodeTarget{PhoneID: 1, Mode: ModeVoice}) || step.Busy || !strings.Contains(step.Notice, "calling") {
		t.Errorf("step %+v", step)
	}
}

func TestReducerTreatsAChallengeWithNoRouteAsUnsupported(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: &services.SecondFactor{CodeLength: 6}})
	if step.Kind != StepCredentials || step.Error == nil || step.Error.Code != services.CodeUnsupported2FA {
		t.Errorf("step %+v", step)
	}
	if step.Error.Message != "No way to receive a code." {
		t.Errorf("message %q", step.Error.Message)
	}
}

func TestReducerKeepsTheCodeStepOpenOnAWrongCode(t *testing.T) {
	step := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
		SignInAction{Type: ActSubmit},
		failed(services.CodeIncorrectCode))
	if step.Kind != StepCode || step.Busy || step.Error == nil || step.Error.Code != services.CodeIncorrectCode {
		t.Errorf("step %+v", step)
	}
	// Network and unknown errors keep the attempt open too.
	for _, code := range []services.ErrorCode{services.CodeNetwork, services.CodeUnknown} {
		if s := run(step, SignInAction{Type: ActSubmit}, failed(code)); s.Kind != StepCode || s.Error.Code != code {
			t.Errorf("%s: %+v", code, s)
		}
	}
}

func TestReducerEndsTheAttemptOnErrorsGoEndsItFor(t *testing.T) {
	for _, code := range []services.ErrorCode{services.CodeLocked, services.CodeActionRequired, services.CodeUnsupported2FA, services.CodeTimeout} {
		step := run(initialSignIn,
			SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
			SignInAction{Type: ActSubmit},
			failed(code))
		if step.Kind != StepCredentials || step.Error == nil || step.Error.Code != code {
			t.Errorf("%s: %+v", code, step)
		}
	}
}

func TestReducerClearsTheErrorAndNoticeWhenTheUserActsAgain(t *testing.T) {
	step := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
		failed(services.CodeIncorrectCode),
		SignInAction{Type: ActResend})
	if step.Kind != StepCode || !step.Busy || step.Error != nil {
		t.Fatalf("step %+v", step)
	}
	resent := signInReducer(step, SignInAction{Type: ActResent})
	if resent.Busy || resent.Notice != "A new code is on its way to your devices." {
		t.Errorf("resent %+v", resent)
	}
	// A new submit clears the notice again.
	if again := signInReducer(resent, SignInAction{Type: ActSubmit}); again.Notice != "" || !again.Busy {
		t.Errorf("submit %+v", again)
	}
	// A phone code says it is on its way without the devices.
	byPhone := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: smsOnly}, SignInAction{Type: ActResend}, SignInAction{Type: ActResent})
	if byPhone.Notice != "A new code is on its way." {
		t.Errorf("phone notice %q", byPhone.Notice)
	}
}

func TestReducerLetsADeviceAccountPickATextOrCallNumberInstead(t *testing.T) {
	step := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
		SignInAction{Type: ActChoosePhone, Mode: ModeVoice})
	if step.Kind != StepChoosePhone || step.Mode != ModeVoice || currentMethod(step) != MethodVoice {
		t.Fatalf("step %+v", step)
	}
	// And back to the code already on the devices.
	back := signInReducer(step, SignInAction{Type: ActUseDevice})
	if back.Kind != StepCode || back.Target.PhoneID != 0 || currentMethod(back) != MethodDevice {
		t.Errorf("back %+v", back)
	}
}

func TestReducerHasNoDeviceRouteWithoutAnAppleDevice(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: phonesOnly})
	if got := signInReducer(step, SignInAction{Type: ActUseDevice}); got != step {
		t.Errorf("use-device changed %+v to %+v", step, got)
	}
}

func TestReducerIgnoresChoosePhoneWithoutPhones(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: &services.SecondFactor{CodeLength: 6, TrustedDevices: true}})
	if got := signInReducer(step, SignInAction{Type: ActChoosePhone, Mode: ModeSMS}); got != step {
		t.Errorf("choose-phone changed %+v to %+v", step, got)
	}
	if got := signInReducer(initialSignIn, SignInAction{Type: ActChoosePhone}); got != initialSignIn {
		t.Errorf("choose-phone on credentials: %+v", got)
	}
}

func TestReducerSwitchesATextedCodeToAnotherNumberKeepingTheMethod(t *testing.T) {
	step := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: smsOnly}, SignInAction{Type: ActChoosePhone})
	if step.Kind != StepChoosePhone || step.Mode != ModeVoice {
		t.Errorf("step %+v", step)
	}
	// From the device code, the number choice starts on text.
	fromDevice := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: deviceFactor}, SignInAction{Type: ActChoosePhone})
	if fromDevice.Mode != ModeSMS {
		t.Errorf("from the device: %+v", fromDevice)
	}
}

func TestReducerFollowsTheAuthStatus(t *testing.T) {
	// A pending challenge from before the form mounted is restored.
	if got := signInReducer(initialSignIn, SignInAction{Type: ActStatus, State: "signing_in", Factor: deviceFactor}); got.Kind != StepCode {
		t.Errorf("restored %+v", got)
	}
	// Go dropped the attempt: back to the form with the reason.
	dropped := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
		SignInAction{Type: ActStatus, State: "signed_out", LastErrorCode: services.CodeTimeout})
	if dropped.Kind != StepCredentials || dropped.Error == nil || dropped.Error.Code != services.CodeTimeout {
		t.Errorf("dropped %+v", dropped)
	}
	// ... or cancelled when Go gave none.
	gone := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
		SignInAction{Type: ActStatus, State: "signed_out"})
	if gone.Error == nil || gone.Error.Code != services.CodeCancelled {
		t.Errorf("gone %+v", gone)
	}
	// Minted: done.
	done := run(initialSignIn,
		SignInAction{Type: ActSecondFactor, Factor: deviceFactor},
		SignInAction{Type: ActSubmit},
		SignInAction{Type: ActStatus, State: "validating"})
	if done.Kind != StepDone {
		t.Errorf("done %+v", done)
	}
	// An idle form stays as it is when someone is already signed in.
	if got := signInReducer(initialSignIn, SignInAction{Type: ActStatus, State: "signed_in"}); got != initialSignIn {
		t.Errorf("idle form %+v", got)
	}
	// A busy one is done.
	busy := run(initialSignIn, SignInAction{Type: ActSubmit}, SignInAction{Type: ActStatus, State: "signed_in"})
	if busy.Kind != StepDone {
		t.Errorf("busy form %+v", busy)
	}
	// A challenge shown is not replaced by a restored one.
	shown := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: deviceFactor})
	if got := signInReducer(shown, SignInAction{Type: ActStatus, State: "signing_in", Factor: smsOnly}); got != shown {
		t.Errorf("replaced %+v", got)
	}
}

func TestReducerCancelReturnsToAnEmptyForm(t *testing.T) {
	got := run(initialSignIn, SignInAction{Type: ActSecondFactor, Factor: deviceFactor}, SignInAction{Type: ActCancel})
	if got != initialSignIn {
		t.Errorf("got %+v", got)
	}
}

func TestReducerSignedInEndsTheForm(t *testing.T) {
	if got := run(initialSignIn, SignInAction{Type: ActSubmit}, SignInAction{Type: ActSignedIn}); got.Kind != StepDone {
		t.Errorf("got %+v", got)
	}
	// Nothing reopens a finished form.
	if got := signInReducer(SignInStep{Kind: StepDone}, SignInAction{Type: ActSubmit}); got.Kind != StepDone || got.Busy {
		t.Errorf("submit after done: %+v", got)
	}
}
