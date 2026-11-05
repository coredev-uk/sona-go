package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// Touch ID unlock (docs/auth.md §7.1): with Config.BiometricOn the Music
// user token lives in Config.Biometric's Keychain item, guarded by Touch ID,
// instead of Open's store. Reading it (Start, RetryKeyring) is the Touch ID
// prompt. Cancelling leaves Sona signed out with Status.Keyring
// KeyringLocked; a change to the enrolled fingerprints, or a missing item,
// ends the unlock (ErrBiometricReset). Passwords and codes are never stored.

// openStore is the vault's opener: the biometric store while Touch ID
// unlock is on, else Config.Open's.
func (m *Manager) openStore() (store.Store, error) {
	if m.biometricOn() {
		s, err := store.OpenBiometric(m.cfg.Biometric)
		if err == nil {
			return s, nil
		}
		m.log.Warn("Touch ID unlock is on but unavailable in this build; using the login Keychain", "err", err)
	}
	return m.cfg.Open()
}

func (m *Manager) biometricConfigured() bool {
	return m.cfg.Biometric != nil && m.cfg.BiometricOn != nil && m.cfg.SetBiometricOn != nil
}

func (m *Manager) biometricOn() bool { return m.biometricConfigured() && m.cfg.BiometricOn() }

// BiometricSupported is nil when Touch ID unlock can be turned on: this
// build has a biometric Keychain and its code signature allows it. Anything
// else wraps errors.ErrUnsupported and says what is missing.
func (m *Manager) BiometricSupported() error {
	if !m.biometricConfigured() {
		return fmt.Errorf("auth: Touch ID unlock isn't available on this system: %w", errors.ErrUnsupported)
	}
	return m.cfg.Biometric.Available()
}

// BiometricEnabled reports whether the token is kept behind Touch ID.
func (m *Manager) BiometricEnabled() bool { return m.biometricOn() }

// SetBiometricUnlock moves the signed-in token into the Touch ID Keychain
// item (on) or back to Open's store (off), and persists the setting. The
// source is deleted only after the destination holds the token. It needs a
// signed-in token and, to turn on, BiometricSupported; the move shows no
// Touch ID prompt, since writing the item needs none.
func (m *Manager) SetBiometricUnlock(ctx context.Context, on bool) error {
	if on {
		if err := m.BiometricSupported(); err != nil {
			return err
		}
	} else if !m.biometricConfigured() {
		return fmt.Errorf("auth: Touch ID unlock isn't available on this system: %w", errors.ErrUnsupported)
	}
	m.persist.Lock()
	defer m.persist.Unlock()
	if m.cfg.BiometricOn() == on {
		return nil
	}
	m.mu.Lock()
	rec := m.record
	m.mu.Unlock()
	if rec.Token == "" {
		return ErrNotSignedIn
	}
	openTo := m.cfg.Open
	if on {
		openTo = func() (store.Store, error) { return store.OpenBiometric(m.cfg.Biometric) }
	}
	res := m.vault.migrate(openTo, func(from, to store.Store) error {
		if err := to.Set(rec); err != nil {
			return err
		}
		if err := m.cfg.SetBiometricOn(on); err != nil {
			return errors.Join(fmt.Errorf("save the setting: %w", err), to.Delete())
		}
		if err := from.Delete(); err != nil {
			m.log.Warn("the previous token store kept its copy", "err", err)
		}
		return nil
	})
	err := await(ctx, res, m.cfg.KeyringSlow, m.cfg.KeyringTimeout, nil)
	if err != nil {
		if !errors.Is(err, ErrKeyring) && !errors.Is(err, errors.ErrUnsupported) {
			err = fmt.Errorf("%w: move token: %w", ErrKeyring, err)
		}
		return err
	}
	m.update(func() bool { return true }) // the Backend shown changed
	m.log.Info("Touch ID unlock changed", "on", on, "backend", m.vault.Backend())
	return nil
}

// leaveBiometric turns the setting off and makes the vault open Open's
// store next. The caller has deleted the Touch ID item.
func (m *Manager) leaveBiometric() error {
	if err := m.cfg.SetBiometricOn(false); err != nil {
		return fmt.Errorf("turn off Touch ID unlock: %w", err)
	}
	m.vault.do(true, func(store.Store) error { return nil })
	return nil
}

// biometricReset ends Touch ID unlock after the stored item became
// unreadable for good (restore id's answer): it deletes the item, turns the
// setting off and reports signed_out with ErrBiometricReset.
func (m *Manager) biometricReset(id uint64, cause error) {
	m.log.Warn("Touch ID can no longer unlock the stored token; signed out", "err", cause)
	del := m.vault.do(false, func(s store.Store) error { return s.Delete() })
	if err := await(m.ctx, del, m.cfg.KeyringSlow, m.cfg.KeyringTimeout, nil); err != nil {
		m.log.Warn("deleting the Touch ID item failed", "err", err)
	}
	if err := m.leaveBiometric(); err != nil {
		m.log.Warn("Touch ID unlock stays on", "err", err)
	}
	m.update(func() bool {
		if m.restoring != id {
			return false
		}
		m.restoring = 0
		m.st = Status{State: StateSignedOut, LastError: "Touch ID unlock ended; sign in again", LastCause: ErrBiometricReset}
		return true
	})
}
