package store

import (
	"errors"
	"fmt"
)

// BackendKeychainBiometric is the macOS data protection Keychain item that
// only Touch ID (the enrolled fingerprints at the time it was written) opens.
const BackendKeychainBiometric Backend = "keychain (Touch ID)"

// Errors returned by a BiometricKeychain's Get, besides ErrNotFound.
var (
	// ErrBiometricDenied: the user cancelled the Touch ID prompt, it failed,
	// or macOS can't show it now. The item is untouched; asking again works.
	ErrBiometricDenied = errors.New("store: Touch ID was cancelled or didn't match")
	// ErrBiometricInvalidated: the enrolled fingerprints changed, so the item
	// can never be read again. Callers treat it as signed out and delete it.
	ErrBiometricInvalidated = errors.New("store: Touch ID's enrolled fingerprints changed")
)

// BiometricKeychain is one Keychain item guarded by a biometric access
// control (SecAccessControl .biometryCurrentSet). macOS shows the Touch ID
// prompt itself, inside Get, which blocks until the user answers. The macOS
// implementation is SystemBiometric; tests inject fakes.
type BiometricKeychain interface {
	// Available is nil when this build can keep biometric items: a macOS
	// run whose code signature carries a team ID and the keychain access
	// groups entitlement. Anything else is an error wrapping
	// errors.ErrUnsupported that says what is missing.
	Available() error
	// Get returns the item's bytes (the Touch ID prompt), ErrNotFound when
	// there is none, ErrBiometricDenied or ErrBiometricInvalidated.
	Get() ([]byte, error)
	// Set replaces the item, with the biometric access control. It shows no
	// prompt.
	Set([]byte) error
	// Delete removes the item; deleting a missing one is not an error. It
	// shows no prompt.
	Delete() error
}

// OpenBiometric returns the Store that keeps the Record in kc, the one
// Settings → "Use Touch ID to unlock Sona" selects over Open's login
// Keychain. It never falls back to a file: a failing item leaves Sona signed
// out, which is the point of the protection.
func OpenBiometric(kc BiometricKeychain) (Store, error) {
	if kc == nil {
		return nil, fmt.Errorf("store: Touch ID isn't available here: %w", errors.ErrUnsupported)
	}
	if err := kc.Available(); err != nil {
		return nil, err
	}
	return recordStore{biometricSlot{kc}}, nil
}

type biometricSlot struct{ kc BiometricKeychain }

func (biometricSlot) backend() Backend { return BackendKeychainBiometric }

func (s biometricSlot) get() ([]byte, error) { return s.kc.Get() }
func (s biometricSlot) set(b []byte) error   { return s.kc.Set(b) }
func (s biometricSlot) del() error           { return s.kc.Delete() }
