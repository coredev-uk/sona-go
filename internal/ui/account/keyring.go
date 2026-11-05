package account

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// KeyringRetry is KeyringRetry.tsx: it reads the stored sign-in from the
// keyring again (AuthService.RetryKeyring) after the keyring didn't answer,
// refused, or wasn't there. While that read waits it stays in place as
// "Trying…" (still focusable, and clicks are ignored); during the start-up
// read nobody asked for, it shows nothing.
type KeyringRetry struct {
	d     Deps
	asked bool
}

// NewKeyringRetry is a KeyringRetry over d.
func NewKeyringRetry(d Deps) *KeyringRetry { return &KeyringRetry{d: d} }

// Build builds the button for status; main thread only.
func (r *KeyringRetry) Build(c *ui.Context, status services.AuthStatus) {
	trying := status.State == "restoring"
	if trying && !r.asked {
		return
	}
	label := i18n.T("Try again")
	if status.Keyring == "locked" {
		label = i18n.T("Try Touch ID again")
	}
	if trying {
		label = i18n.T("Trying…")
	}
	opts := kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM}
	if trying {
		// aria-disabled: MyGo has no focusable-but-disabled state, so the
		// button stays enabled (the focus stays on it), ignores the press and
		// says why.
		opts.Reason = waitingWords(r.d.OS, status)
	}
	if kit.Button(c.Key("keyring-retry"), label, opts).Clicked() && !trying {
		r.asked = true
		api := r.d.API
		go api.RetryKeyring()
	}
}

// keyringNotice is where the keyring holds sign-in up: the stored sign-in
// still being read, a keyring that didn't answer or failed, or no keyring at
// all (a sign-in would go to the unencrypted file). The retry keeps its
// place across these, so focus stays on it while it reads again.
func (s *Section) keyringNotice(c *ui.Context, st services.AuthStatus) {
	restoring := st.State == "restoring"
	if restoring {
		if st.Keyring != "waiting" {
			return
		}
	} else if st.Keyring != "unavailable" && st.Keyring != "locked" && !st.Unencrypted {
		return
	}
	retry := func() { s.retry.Build(c, st) }
	if restoring {
		notice(c, noticeOpts{Spin: true}, func() {
			noticeTitle(c, i18n.T("Signing in…"))
			noticeDetail(c, waitingWords(s.d.OS, st))
		}, retry)
		return
	}
	// A problem found on arrival, not caused by the user's last action: a status, not an alert.
	notice(c, noticeOpts{Icon: "shield-alert", IconColor: "danger", Problem: true}, func() {
		if st.Keyring == "locked" {
			noticeTitle(c, i18n.T("Touch ID didn't unlock Sona"))
			noticeDetail(c, i18n.T("You're signed out until it does. Try Touch ID again, or sign in with your Apple Account."))
			return
		}
		if st.Keyring == "unavailable" {
			noticeTitle(c, i18n.T("Sona couldn't read your sign-in from your keyring"))
			noticeDetail(c, kit.KeyringWords(s.d.OS).Refused)
			return
		}
		noticeTitle(c, i18n.T("Signing in now stores your token unencrypted"))
		noticeDetail(c, kit.KeyringWords(s.d.OS).NotOpened)
	}, retry)
}

// waitingWords is what a stored sign-in being read waits for: the Touch ID
// prompt when the token is kept behind it, else the keyring.
func waitingWords(goos string, st services.AuthStatus) string {
	if st.BiometricUnlock {
		return i18n.T("Sona is waiting for Touch ID to unlock your sign-in.")
	}
	return kit.KeyringWords(goos).Waiting
}
