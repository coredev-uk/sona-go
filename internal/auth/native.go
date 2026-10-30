package auth

import (
	"context"
	"errors"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/mut"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// Native sign-in (docs/auth.md §4.7): Sona's own form collects the Apple
// Account name, password and two-factor code, and package appleid speaks
// Apple's sign-in protocol. An attempt spans several calls:
// SignInWithPassword, then (when Apple asks for one) SubmitSecurityCode,
// optionally after SendCodeToPhone or ResendSecurityCode. While Apple
// waits for a code the state is signing_in with Status.SecondFactor set.
// The attempt ends at SignInTimeout, on CancelSignIn, SignOut or a newer
// attempt.

// nativeAttempt is the pending two-factor step of a native sign-in.
type nativeAttempt struct {
	id   uint64
	ctx  context.Context // ends with the attempt
	end  func()          // cancels ctx and releases the attempt
	sess *appleid.Session
}

// SignInWithPassword starts a native sign-in. It returns nil when Apple
// signed the account in and the token is stored, or the two-factor
// challenge to answer with SubmitSecurityCode. On any other failure the
// previous state returns and the error is reported in Status.LastError.
// The password is used for one exchange and not kept.
func (m *Manager) SignInWithPassword(ctx context.Context, account, password string) (*appleid.SecondFactor, error) {
	actx, id, done := m.beginAttempt(context.WithoutCancel(ctx), StateSigningIn)
	actx, timeout := context.WithTimeoutCause(actx, m.signInTimeout(), mut.ErrTimeout)
	step, stop := joinContext(ctx, actx)
	defer stop()

	end := func() { timeout(); done() }
	// fail reads the attempt's cause before ending it.
	fail := func(err error) error {
		err = m.failAttempt(id, nativeError(attemptErr(actx, err)))
		end()
		return err
	}
	tok, err := m.cfg.Dev.Token(step)
	if err != nil {
		return nil, fail(err)
	}
	sess, err := appleid.Begin(step, tok.JWT, m.appleIDOptions())
	if err != nil {
		return nil, fail(err)
	}
	sf, err := sess.SignIn(step, account, password)
	if err != nil {
		sess.Close()
		return nil, fail(err)
	}
	if sf == nil {
		defer end()
		defer sess.Close()
		return nil, m.completeNative(step, id, sess)
	}

	na := &nativeAttempt{id: id, ctx: actx, end: end, sess: sess}
	if !m.update(func() bool {
		if m.attempt != id {
			return false
		}
		m.native = na
		m.st.SecondFactor = sf
		return true
	}) {
		sess.Close()
		end()
		return nil, mut.ErrCanceled
	}
	// Watch the attempt: drop the session when it ends however it ends,
	// and report the deadline.
	go func() {
		<-actx.Done()
		sess.Close()
		m.update(func() bool {
			if m.native != na {
				return false
			}
			m.native = nil
			return false // no visible change
		})
		if errors.Is(context.Cause(actx), mut.ErrTimeout) {
			m.failAttempt(id, mut.ErrTimeout)
		}
		end()
	}()
	m.log.Info("sign-in needs a verification code", "trustedDevices", sf.TrustedDevices, "phones", len(sf.Phones))
	return sf, nil
}

// SubmitSecurityCode answers the pending two-factor challenge: phoneID 0
// for a code shown on a trusted device, otherwise the phone the code was
// sent to (mode "sms" or "voice"). A wrong code returns an error wrapping
// appleid.ErrIncorrectCode and keeps the challenge open; other failures
// end the attempt. On success the token is validated and stored.
func (m *Manager) SubmitSecurityCode(ctx context.Context, code string, phoneID int, mode string) error {
	na, err := m.pendingNative()
	if err != nil {
		return err
	}
	step, stop := joinContext(ctx, na.ctx)
	defer stop()
	if err := na.sess.VerifyCode(step, code, phoneID, mode); err != nil {
		err = nativeError(attemptErr(na.ctx, err))
		if errors.Is(err, appleid.ErrIncorrectCode) {
			m.noteNative(na, err)
			return err
		}
		defer na.end()
		return m.failAttempt(na.id, err)
	}
	defer na.end()
	return m.completeNative(step, na.id, na.sess)
}

// SendCodeToPhone asks Apple to send the code to a trusted phone instead.
// Failures (e.g. too many codes) keep the challenge open.
func (m *Manager) SendCodeToPhone(ctx context.Context, phoneID int, mode string) error {
	return m.nativeStep(ctx, func(ctx context.Context, s *appleid.Session) error {
		return s.SendCodeToPhone(ctx, phoneID, mode)
	})
}

// ResendSecurityCode pushes a new code to the trusted devices.
func (m *Manager) ResendSecurityCode(ctx context.Context) error {
	return m.nativeStep(ctx, func(ctx context.Context, s *appleid.Session) error {
		return s.ResendDeviceCode(ctx)
	})
}

func (m *Manager) nativeStep(ctx context.Context, f func(context.Context, *appleid.Session) error) error {
	na, err := m.pendingNative()
	if err != nil {
		return err
	}
	step, stop := joinContext(ctx, na.ctx)
	defer stop()
	if err := f(step, na.sess); err != nil {
		err = nativeError(attemptErr(na.ctx, err))
		m.noteNative(na, err)
		return err
	}
	m.noteNative(na, nil)
	return nil
}

func (m *Manager) pendingNative() (*nativeAttempt, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.native == nil || m.native.id != m.attempt || m.cancelAttempt == nil {
		return nil, appleid.ErrOutOfOrder
	}
	return m.native, nil
}

// noteNative shows (or clears) a recoverable error on the pending attempt.
func (m *Manager) noteNative(na *nativeAttempt, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
		m.log.Warn("sign-in step failed", "err", err)
	}
	m.update(func() bool {
		if m.native != na || m.attempt != na.id || m.st.LastError == msg {
			return false
		}
		m.st.LastError = msg
		return true
	})
}

// completeNative exchanges the authenticated session for a Music User
// Token and finishes the attempt.
func (m *Manager) completeNative(ctx context.Context, id uint64, sess *appleid.Session) error {
	res, err := sess.MusicUserToken(ctx)
	if err != nil {
		return m.failAttempt(id, nativeError(err))
	}
	m.log.Info("native sign-in completed", "mut", res)
	return m.finish(ctx, id, store.Record{Token: res.MusicUserToken, ObtainedAt: time.Now(), Restricted: res.Restricted})
}

func (m *Manager) signInTimeout() time.Duration {
	if m.cfg.SignInTimeout > 0 {
		return m.cfg.SignInTimeout
	}
	return 10 * time.Minute
}

func (m *Manager) appleIDOptions() appleid.Options {
	o := m.cfg.AppleID
	if o.HTTP == nil {
		o.HTTP = m.cfg.HTTP
	}
	return o
}

// joinContext returns a context that ends when either a or b does.
func joinContext(a, b context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(a)
	stop := context.AfterFunc(b, func() { cancel(context.Cause(b)) })
	return ctx, func() { stop(); cancel(context.Canceled) }
}

// attemptErr reports the attempt's own end (deadline, cancel) instead of
// the transport error it caused.
func attemptErr(attempt context.Context, err error) error {
	if cause := context.Cause(attempt); cause != nil {
		if errors.Is(cause, mut.ErrTimeout) {
			return mut.ErrTimeout
		}
		return mut.ErrCanceled
	}
	return err
}

// userError carries a message for Status.LastError while keeping err in
// the chain.
type userError struct {
	msg string
	err error
}

func (e *userError) Error() string { return e.msg }
func (e *userError) Unwrap() error { return e.err }

// nativeError gives appleid failures the wording shown in the UI. Apple's
// own message is preferred where it sent one.
func nativeError(err error) error {
	var se *appleid.ServiceError
	switch {
	case errors.Is(err, mut.ErrCanceled), errors.Is(err, mut.ErrTimeout), errors.Is(err, context.Canceled):
		return err
	case errors.As(err, &se) && se.Message != "":
		return &userError{se.Message, err}
	case errors.Is(err, appleid.ErrInvalidCredentials):
		return &userError{"Incorrect Apple Account or password.", err}
	case errors.Is(err, appleid.ErrIncorrectCode):
		return &userError{"Incorrect verification code.", err}
	case errors.Is(err, appleid.ErrLocked):
		return &userError{"This Apple Account is locked or temporarily unavailable.", err}
	case errors.Is(err, appleid.ErrActionRequired):
		return &userError{"This Apple Account needs attention. Sign in at account.apple.com, then try again.", err}
	case errors.Is(err, appleid.ErrNoSubscription):
		return &userError{"An Apple Music subscription is required.", err}
	case errors.Is(err, appleid.ErrUnsupportedCountry):
		return &userError{"Apple Music is not available in this account's country.", err}
	case errors.Is(err, appleid.ErrUnsupported2FA):
		return &userError{"This account's two-factor method is not supported. Use browser sign-in instead.", err}
	}
	return &userError{"Sign-in failed: " + err.Error(), err}
}
