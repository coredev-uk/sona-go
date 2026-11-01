package services

import (
	"context"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// AuthService is the frontend's sign-in surface over auth.Manager.
type AuthService struct {
	m      *auth.Manager
	emit   Emitter
	openFn func(url string) error

	mu sync.Mutex
	// lastErr is the message and code of the latest failed call, so the
	// status can classify LastError without parsing it.
	lastMsg, lastCode string
}

// newAuthService wires the auth manager to the frontend: every transition
// is logged and emitted as "auth:changed".
func newAuthService(e *Env) *AuthService {
	s := &AuthService{m: e.Auth, emit: e.Emit, openFn: e.OpenURL}
	prev := e.Auth.Status().State
	e.Logger.Info("auth: state", "to", prev)
	e.Auth.OnChange(func(st auth.Status) {
		if st.State != prev {
			e.Logger.Info("auth: state", "from", prev, "to", st.State, "offline", st.Offline)
			prev = st.State
		}
		s.emit.Emit(EventAuthChanged, s.status(st))
	})
	return s
}

// Status returns the current sign-in state.
func (s *AuthService) Status() AuthStatus { return s.status(s.m.Status()) }

func (s *AuthService) status(st auth.Status) AuthStatus {
	out := AuthStatus{
		State:          string(st.State),
		StorefrontID:   st.StorefrontID,
		StorefrontName: st.StorefrontName,
		Language:       st.Language,
		Restricted:     st.Restricted,
		Backend:        string(st.Backend),
		Unencrypted:    st.Backend == store.BackendFile,
		Keyring:        string(st.Keyring),
		Offline:        st.Offline,
		LastError:      st.LastError,
		BrowserSignIn:  st.BrowserSignIn,
		SecondFactor:   secondFactorDTO(st.SecondFactor),
	}
	out.LastErrorCode = s.codeFor(st)
	return out
}

// codeFor classifies st.LastError: by the failed call that produced it, or
// by the state the manager reached on its own.
func (s *AuthService) codeFor(st auth.Status) string {
	if st.LastError == "" {
		return ""
	}
	s.mu.Lock()
	msg, code := s.lastMsg, s.lastCode
	s.mu.Unlock()
	switch {
	case code != "" && (st.LastError == msg || strings.Contains(msg, st.LastError)):
		return code
	case st.Keyring == auth.KeyringUnavailable:
		return CodeKeyring
	case st.State == auth.StateNeedsReauth:
		return CodeMUTRejected
	case st.State == auth.StateError && strings.Contains(st.LastError, "developer token"):
		return CodeDevTokenRejected
	}
	return CodeUnknown
}

// note records a failed call's classification. The manager has already
// emitted the status reporting it (unclassified), so it is emitted again.
func (s *AuthService) note(err error) error {
	if err == nil {
		return nil
	}
	s.mu.Lock()
	s.lastMsg, s.lastCode = err.Error(), ErrorCode(err)
	s.mu.Unlock()
	s.emit.Emit(EventAuthChanged, s.Status())
	return err
}

// SignInWithPassword starts the native sign-in. It returns nil when Apple
// signed the account in directly, or the two-factor challenge to answer
// with SubmitSecurityCode. The password is never stored or logged.
func (s *AuthService) SignInWithPassword(ctx context.Context, account, password string) (*SecondFactor, error) {
	sf, err := s.m.SignInWithPassword(ctx, account, password)
	if err != nil {
		return nil, s.note(err)
	}
	return secondFactorDTO(sf), nil
}

// SubmitSecurityCode completes a pending native sign-in. phoneID 0 submits
// a trusted-device code; mode is the phone's "sms" or "voice".
func (s *AuthService) SubmitSecurityCode(ctx context.Context, code string, phoneID int, mode string) error {
	return s.note(s.m.SubmitSecurityCode(ctx, code, phoneID, mode))
}

// SendCodeToPhone asks Apple to send a code to a trusted phone.
func (s *AuthService) SendCodeToPhone(ctx context.Context, phoneID int, mode string) error {
	return s.note(s.m.SendCodeToPhone(ctx, phoneID, mode))
}

// ResendSecurityCode re-sends the trusted-device code.
func (s *AuthService) ResendSecurityCode(ctx context.Context) error {
	return s.note(s.m.ResendSecurityCode(ctx))
}

// SignIn runs the browser sign-in fallback: it opens the system browser
// and resolves when the sign-in completes, fails or is cancelled.
func (s *AuthService) SignIn(ctx context.Context) error {
	// The attempt outlives a reload of the page that started it; it ends
	// with CancelSignIn, SignOut, a newer attempt or the deadline.
	return s.note(s.m.SignIn(context.WithoutCancel(ctx), s.openFn))
}

// SignInLink returns a new single-use link to the pending browser sign-in,
// for opening the page again or in another browser.
func (s *AuthService) SignInLink() (string, error) { return s.m.SignInLink() }

// CancelSignIn abandons a pending sign-in.
func (s *AuthService) CancelSignIn() { s.m.CancelSignIn() }

// SignInWithToken validates and stores a pasted Music User Token.
func (s *AuthService) SignInWithToken(ctx context.Context, token string) error {
	return s.note(s.m.SignInWithToken(ctx, strings.TrimSpace(token)))
}

// SignOut deletes the stored token and all user data.
func (s *AuthService) SignOut(ctx context.Context) error {
	return s.note(s.m.SignOut(context.WithoutCancel(ctx)))
}

// Revalidate checks the stored token with Apple now (the UI's retry).
func (s *AuthService) Revalidate(ctx context.Context) error {
	return s.note(s.m.Revalidate(ctx))
}

// RetryKeyring reads the stored sign-in again after the keyring was
// unavailable (AuthStatus.Keyring "unavailable", or the unencrypted file
// fallback while signed out); the outcome arrives as auth:changed.
func (s *AuthService) RetryKeyring() { s.m.RetryKeyring() }
