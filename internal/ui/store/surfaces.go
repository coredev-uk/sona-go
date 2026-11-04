package store

// The first-run setup's steps, in order. A step name is what OpenSetup takes
// and Surfaces.SetupStep returns; an unknown one means SetupWelcome.
const (
	SetupWelcome     = "welcome"
	SetupAccount     = "account"
	SetupPlayback    = "playback"
	SetupConnections = "connections"
	SetupAppearance  = "appearance"
	SetupDesktop     = "desktop"
	SetupDone        = "done"
)

// Surfaces is the open state of the two surfaces any feature may ask for:
// the account dialog (features/auth) and the first-run setup layer
// (features/onboarding). v0.2.0's shared/lib/account.ts and setup.ts, so
// every "Sign in" action and Settings → General's "Set up Sona again" can
// open them without importing the feature that draws them.
//
// Main thread only: it is read while a view builds and written by what the
// user did (a click, a shortcut), never from a goroutine.
type Surfaces struct {
	account bool
	setup   bool
	step    string
}

// NewSurfaces is a Surfaces with both closed.
func NewSurfaces() *Surfaces { return &Surfaces{} }

// OpenAccount shows the account dialog. While the setup is open it goes to
// the setup's Apple Music step instead (the account is part of the setup).
func (s *Surfaces) OpenAccount() {
	if s.setup {
		s.step = SetupAccount
		return
	}
	s.account = true
}

// CloseAccount hides the account dialog.
func (s *Surfaces) CloseAccount() { s.account = false }

// AccountOpen reports whether the account dialog is open.
func (s *Surfaces) AccountOpen() bool { return s.account }

// OpenSetup opens the first-run setup at a step (SetupWelcome… SetupDone; ""
// or an unknown name is the welcome). Opening it while it is open only
// jumps to the step. The account dialog closes: the setup covers the window.
func (s *Surfaces) OpenSetup(step string) {
	s.setup = true
	s.account = false
	s.step = step
	if !validSetupStep(step) {
		s.step = SetupWelcome
	}
}

// CloseSetup hides the setup.
func (s *Surfaces) CloseSetup() { s.setup = false }

// SetupOpen reports whether the setup is open.
func (s *Surfaces) SetupOpen() bool { return s.setup }

// SetupStep is the step the setup shows (SetupWelcome when it never opened).
func (s *Surfaces) SetupStep() string {
	if !validSetupStep(s.step) {
		return SetupWelcome
	}
	return s.step
}

// SetSetupStep moves the open setup to a step.
func (s *Surfaces) SetSetupStep(step string) {
	if validSetupStep(step) {
		s.step = step
	}
}

func validSetupStep(step string) bool {
	switch step {
	case SetupWelcome, SetupAccount, SetupPlayback, SetupConnections, SetupAppearance, SetupDesktop, SetupDone:
		return true
	}
	return false
}
