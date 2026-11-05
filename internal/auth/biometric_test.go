package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// fakeBio is the Touch ID Keychain item as macOS behaves, without a Mac: Get
// is the prompt, which the user can cancel (deny), and a change to the
// enrolled fingerprints makes the item read as missing (invalidate).
type fakeBio struct {
	mu          sync.Mutex
	unsupported error
	item        []byte
	deny        bool
	gets        int
}

func (f *fakeBio) Available() error { return f.unsupported }

func (f *fakeBio) Get() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.item == nil {
		return nil, store.ErrNotFound
	}
	if f.deny {
		return nil, store.ErrBiometricDenied
	}
	return append([]byte(nil), f.item...), nil
}

func (f *fakeBio) Set(b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.item = append([]byte(nil), b...)
	return nil
}

func (f *fakeBio) Delete() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.item = nil
	return nil
}

func (f *fakeBio) set(deny bool) { f.mu.Lock(); f.deny = deny; f.mu.Unlock() }
func (f *fakeBio) invalidate()   { f.mu.Lock(); f.item = nil; f.mu.Unlock() }
func (f *fakeBio) has() bool     { f.mu.Lock(); defer f.mu.Unlock(); return f.item != nil }
func (f *fakeBio) count() int    { f.mu.Lock(); defer f.mu.Unlock(); return f.gets }

// touchEnv is an env whose manager has Touch ID unlock available.
func touchEnv(t *testing.T) (*env, *fakeBio) {
	e := newEnv(t)
	bio := &fakeBio{}
	e.bio, e.bioOn = bio, &atomic.Bool{}
	return e, bio
}

// enable signs in from the plain store and turns Touch ID unlock on.
func enable(t *testing.T, e *env, bio *fakeBio) (*Manager, *recorder) {
	t.Helper()
	e.stored(goodMUT, true)
	m, rec := e.manager(t)
	if s := start(t, m, rec); s.State != StateSignedIn {
		t.Fatalf("start: %+v", s)
	}
	if err := m.SetBiometricUnlock(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	return m, rec
}

func TestTouchIDEnableMovesTheToken(t *testing.T) {
	e, bio := touchEnv(t)
	m, _ := enable(t, e, bio)
	if !bio.has() || e.store.token() != "" || !e.bioOn.Load() {
		t.Fatalf("item=%v plain=%q on=%v", bio.has(), e.store.token(), e.bioOn.Load())
	}
	if s := m.Status(); s.State != StateSignedIn || s.Backend != store.BackendKeychainBiometric || m.UserToken() != goodMUT {
		t.Fatalf("status = %+v", s)
	}
	if bio.count() != 0 {
		t.Fatal("enabling showed the Touch ID prompt")
	}

	if err := m.SetBiometricUnlock(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if bio.has() || e.store.token() != goodMUT || e.bioOn.Load() || m.Status().Backend != store.BackendSecretService {
		t.Fatalf("after off: item=%v plain=%q on=%v backend=%v", bio.has(), e.store.token(), e.bioOn.Load(), m.Status().Backend)
	}
}

func TestTouchIDStartUnlocks(t *testing.T) {
	e, bio := touchEnv(t)
	enable(t, e, bio)

	m, rec := e.manager(t) // a new start with the setting on and the item stored
	s := start(t, m, rec)
	if s.State != StateSignedIn || s.Backend != store.BackendKeychainBiometric || m.UserToken() != goodMUT {
		t.Fatalf("after unlock: %+v", s)
	}
	if bio.count() != 1 {
		t.Fatalf("Touch ID prompts = %d, want 1", bio.count())
	}
}

func TestTouchIDCancelSignsOutAndRetryAsksAgain(t *testing.T) {
	e, bio := touchEnv(t)
	enable(t, e, bio)
	bio.set(true)

	m, rec := e.manager(t)
	s := start(t, m, rec)
	if s.State != StateSignedOut || s.Keyring != KeyringLocked || m.UserToken() != "" {
		t.Fatalf("after cancel: %+v", s)
	}
	if !bio.has() || !e.bioOn.Load() {
		t.Fatal("cancelling dropped the item or the setting")
	}

	bio.set(false)
	m.RetryKeyring()
	s = rec.wait(t, func(s Status) bool { return s.State != StateRestoring })
	if s.State != StateSignedIn || s.Keyring != KeyringReady || m.UserToken() != goodMUT {
		t.Fatalf("after Try Touch ID again: %+v", s)
	}
}

func TestTouchIDInvalidatedSignsOut(t *testing.T) {
	e, bio := touchEnv(t)
	enable(t, e, bio)
	bio.invalidate() // the enrolled fingerprints changed

	m, rec := e.manager(t)
	s := start(t, m, rec)
	if s.State != StateSignedOut || !errors.Is(s.LastCause, ErrBiometricReset) || s.Keyring != KeyringReady {
		t.Fatalf("after invalidation: %+v", s)
	}
	if e.bioOn.Load() {
		t.Fatal("Touch ID unlock stayed on")
	}

	// Signing in again keeps the token in the login Keychain, not the item.
	if err := m.SignInWithToken(context.Background(), goodMUT); err != nil {
		t.Fatal(err)
	}
	if e.store.token() != goodMUT || bio.has() || m.Status().Backend != store.BackendSecretService {
		t.Fatalf("plain=%q item=%v backend=%v", e.store.token(), bio.has(), m.Status().Backend)
	}
}

func TestTouchIDSignOutDeletesTheItem(t *testing.T) {
	e, bio := touchEnv(t)
	m, _ := enable(t, e, bio)
	if err := m.SignOut(context.Background()); err != nil {
		t.Fatal(err)
	}
	if bio.has() || e.bioOn.Load() {
		t.Fatalf("after sign-out: item=%v on=%v", bio.has(), e.bioOn.Load())
	}
	if err := m.SignInWithToken(context.Background(), goodMUT); err != nil {
		t.Fatal(err)
	}
	if bio.has() || e.store.token() != goodMUT {
		t.Fatal("the next sign-in didn't go back to the login Keychain")
	}
}

func TestTouchIDUnsupported(t *testing.T) {
	e, bio := touchEnv(t)
	bio.unsupported = errors.Join(errors.ErrUnsupported, errors.New("no team ID"))
	e.stored(goodMUT, true)
	m, rec := e.manager(t)
	start(t, m, rec)

	if m.BiometricSupported() == nil {
		t.Fatal("reported supported")
	}
	if err := m.SetBiometricUnlock(context.Background(), true); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("enable = %v, want ErrUnsupported", err)
	}
	if e.bioOn.Load() || bio.has() || e.store.token() != goodMUT {
		t.Fatal("an unsupported build changed where the token is kept")
	}
}

func TestTouchIDWithoutAKeychain(t *testing.T) {
	e := newEnv(t) // Linux and Windows: no Config.Biometric
	m, _ := e.manager(t)
	if err := m.SetBiometricUnlock(context.Background(), true); !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("enable = %v, want ErrUnsupported", err)
	}
}
