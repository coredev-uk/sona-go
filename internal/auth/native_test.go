package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/appleid"
	"github.com/coredev-uk/sona-go/internal/auth/appleid/appleidtest"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

func (e *env) nativeManager(t *testing.T, cfg appleidtest.Config, timeout time.Duration) (*Manager, *recorder) {
	t.Helper()
	idms := appleidtest.New(t, cfg)
	m, err := NewManager(Config{
		HTTP: e.apple.srv.Client(), Dev: e.dev, Open: func() (store.Store, error) { return e.store, nil }, Accounts: e.accounts,
		APIBase: e.apple.srv.URL, AppleID: idms.Options(), SignInTimeout: timeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	r := &recorder{ch: make(chan Status, 256)}
	m.OnChange(func(s Status) { r.ch <- s })
	start(t, m, r)
	return m, r
}

func TestNativeSignInWithTwoFactor(t *testing.T) {
	e := newEnv(t)
	m, r := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT, TwoFactor: true, Restricted: true}, time.Minute)
	ctx := context.Background()

	sf, err := m.SignInWithPassword(ctx, "a@example.com", "pw")
	if err != nil || sf == nil {
		t.Fatalf("SignInWithPassword = %v, %v", sf, err)
	}
	if s := m.Status(); s.State != StateSigningIn || s.SecondFactor == nil || !s.SecondFactor.TrustedDevices {
		t.Fatalf("pending status = %+v", s)
	}

	err = m.SubmitSecurityCode(ctx, "999999", 0, "")
	if !errors.Is(err, appleid.ErrIncorrectCode) {
		t.Fatalf("wrong code: %v", err)
	}
	if s := m.Status(); s.State != StateSigningIn || s.SecondFactor == nil || s.LastError != "Incorrect verification code." {
		t.Fatalf("after wrong code = %+v", s)
	}

	if err := m.SubmitSecurityCode(ctx, appleidtest.Code, 0, ""); err != nil {
		t.Fatal(err)
	}
	s := r.wait(t, is(StateSignedIn))
	if s.SecondFactor != nil || s.StorefrontID != "gb" || !s.Restricted {
		t.Fatalf("signed in status = %+v", s)
	}
	if e.store.token() != goodMUT || m.UserToken() != goodMUT {
		t.Fatal("token not stored")
	}
	if err := m.SubmitSecurityCode(ctx, appleidtest.Code, 0, ""); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("code after sign-in: %v", err)
	}
}

// Accounts without an Apple device get the code by text or call.
func TestNativeSignInWithoutDevicesByPhoneCall(t *testing.T) {
	e := newEnv(t)
	m, r := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT, TwoFactor: true, NoDevices: true, NoAutoSend: true}, time.Minute)
	ctx := context.Background()
	sf, err := m.SignInWithPassword(ctx, "a@example.com", "pw")
	if err != nil || sf == nil || sf.TrustedDevices || len(sf.Phones) != 2 {
		t.Fatalf("SignInWithPassword = %+v, %v", sf, err)
	}
	// A device code is not a route for this account; the attempt stays open.
	if err := m.SubmitSecurityCode(ctx, appleidtest.Code, 0, ""); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("device code: %v", err)
	}
	if err := m.SendCodeToPhone(ctx, 2, "voice"); err != nil {
		t.Fatal(err)
	}
	if s := m.Status(); s.State != StateSigningIn || s.LastError != "" {
		t.Fatalf("after call = %+v", s)
	}
	if err := m.SubmitSecurityCode(ctx, appleidtest.Code, 2, "voice"); err != nil {
		t.Fatal(err)
	}
	r.wait(t, is(StateSignedIn))
	if e.store.token() != goodMUT {
		t.Fatal("token not stored")
	}
}

func TestNativeSignInWithoutTwoFactor(t *testing.T) {
	e := newEnv(t)
	m, _ := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT}, time.Minute)
	sf, err := m.SignInWithPassword(context.Background(), "a@example.com", "pw")
	if err != nil || sf != nil {
		t.Fatalf("SignInWithPassword = %v, %v", sf, err)
	}
	if s := m.Status(); s.State != StateSignedIn {
		t.Fatalf("status = %+v", s)
	}
}

func TestNativeWrongPasswordRestoresState(t *testing.T) {
	e := newEnv(t)
	m, _ := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT}, time.Minute)
	_, err := m.SignInWithPassword(context.Background(), "a@example.com", "bad")
	if !errors.Is(err, appleid.ErrInvalidCredentials) {
		t.Fatalf("err = %v", err)
	}
	s := m.Status()
	if s.State != StateSignedOut || s.LastError != "Check the account information you entered and try again." {
		t.Fatalf("status = %+v", s)
	}
	if e.store.token() != "" {
		t.Fatal("token stored")
	}
}

func TestNativeNoSubscriptionDiscardsSession(t *testing.T) {
	e := newEnv(t)
	m, _ := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", NoSubscription: true}, time.Minute)
	_, err := m.SignInWithPassword(context.Background(), "a@example.com", "pw")
	if !errors.Is(err, appleid.ErrNoSubscription) {
		t.Fatalf("err = %v", err)
	}
	if s := m.Status(); s.State != StateSignedOut || s.LastError != "An Apple Music subscription is required." {
		t.Fatalf("status = %+v", s)
	}
}

func TestNativeCancelDuringTwoFactor(t *testing.T) {
	e := newEnv(t)
	m, _ := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT, TwoFactor: true}, time.Minute)
	if _, err := m.SignInWithPassword(context.Background(), "a@example.com", "pw"); err != nil {
		t.Fatal(err)
	}
	m.CancelSignIn()
	if s := m.Status(); s.State != StateSignedOut || s.SecondFactor != nil || s.LastError != "" {
		t.Fatalf("status = %+v", s)
	}
	if err := m.SubmitSecurityCode(context.Background(), appleidtest.Code, 0, ""); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("code after cancel: %v", err)
	}
}

func TestNativeTwoFactorTimesOut(t *testing.T) {
	e := newEnv(t)
	m, r := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT, TwoFactor: true}, 200*time.Millisecond)
	if _, err := m.SignInWithPassword(context.Background(), "a@example.com", "pw"); err != nil {
		t.Fatal(err)
	}
	s := r.wait(t, func(s Status) bool { return s.State == StateSignedOut && s.LastError != "" })
	if s.LastError != "sign-in timed out" || s.SecondFactor != nil {
		t.Fatalf("status = %+v", s)
	}
	if err := m.SubmitSecurityCode(context.Background(), appleidtest.Code, 0, ""); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("code after timeout: %v", err)
	}
}

func TestNewSignInSupersedesTwoFactor(t *testing.T) {
	e := newEnv(t)
	m, _ := e.nativeManager(t, appleidtest.Config{Account: "a@example.com", Password: "pw", MUT: goodMUT, TwoFactor: true}, time.Minute)
	ctx := context.Background()
	if _, err := m.SignInWithPassword(ctx, "a@example.com", "pw"); err != nil {
		t.Fatal(err)
	}
	if err := m.SignInWithToken(ctx, goodMUT); err != nil {
		t.Fatal(err)
	}
	if s := m.Status(); s.State != StateSignedIn || s.SecondFactor != nil {
		t.Fatalf("status = %+v", s)
	}
	if err := m.SubmitSecurityCode(ctx, appleidtest.Code, 0, ""); !errors.Is(err, appleid.ErrOutOfOrder) {
		t.Fatalf("stale code accepted: %v", err)
	}
}
