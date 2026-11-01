package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/auth/appleid"
)

// EventAuthChanged carries AuthStatus on every sign-in transition.
const EventAuthChanged = "auth:changed"

func init() { application.RegisterEvent[AuthStatus](EventAuthChanged) }

// AuthStatus is the sign-in state ("auth:changed" payload).
type AuthStatus struct {
	// State is restoring, signed_out, signing_in, validating, signed_in,
	// needs_reauth or error.
	State          string `json:"state"`
	StorefrontID   string `json:"storefrontId"`
	StorefrontName string `json:"storefrontName"` // "" until Apple confirmed the token this run
	Language       string `json:"language"`
	Restricted     bool   `json:"restricted"`
	// Backend is where the token is stored ("" until the store has been
	// opened); Unencrypted is set for the 0600 file fallback, which the UI
	// must flag.
	Backend     string `json:"backend"`
	Unencrypted bool   `json:"unencrypted"`
	// Keyring is "waiting" while a read (restoring) or a sign-in's write
	// has waited past a second for the keyring, and from the start of a
	// RetryKeyring read; "unavailable" when the stored sign-in couldn't be
	// read (signed_out; RetryKeyring tries again); "" otherwise.
	Keyring   string `json:"keyring"`
	Offline   bool   `json:"offline"`
	LastError string `json:"lastError"`
	// LastErrorCode classifies LastError (see the Code constants); "" when
	// LastError is empty.
	LastErrorCode string `json:"lastErrorCode"`
	// SignInURL is the browser sign-in start page while a browser sign-in
	// waits, for "open again" / copy link.
	SignInURL string `json:"signInUrl"`
	// SecondFactor is set while a native sign-in waits for a code.
	SecondFactor *SecondFactor `json:"secondFactor"`
}

// SecondFactor describes a pending two-factor challenge.
type SecondFactor struct {
	CodeLength int `json:"codeLength"`
	// TrustedDevices: Apple pushed a code to the account's devices; submit
	// it with phoneId 0.
	TrustedDevices bool    `json:"trustedDevices"`
	Phones         []Phone `json:"phones"`
	SentToPhone    int     `json:"sentToPhone"` // phone Apple already texted, or 0
}

// Phone is a trusted phone number that can receive a code.
type Phone struct {
	ID     int    `json:"id"`
	Number string `json:"number"` // obfuscated by Apple
	Mode   string `json:"mode"`   // "sms" or "voice"
}

func secondFactorDTO(sf *appleid.SecondFactor) *SecondFactor {
	if sf == nil {
		return nil
	}
	out := &SecondFactor{CodeLength: sf.CodeLength, TrustedDevices: sf.TrustedDevices, SentToPhone: sf.SentToPhone, Phones: []Phone{}}
	for _, p := range sf.Phones {
		out.Phones = append(out.Phones, Phone{ID: p.ID, Number: p.Number, Mode: p.Mode})
	}
	return out
}
