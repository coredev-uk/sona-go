package config

// Unlock is how Sona's stored sign-in is unlocked (Settings → Account).
// Every value is valid, so there is nothing to normalise.
type Unlock struct {
	// TouchID keeps the Music user token in a Keychain item that only Touch
	// ID opens (macOS, a signed build: docs/auth.md §7.1). The auth
	// manager flips it with the token's move, so it is true exactly while
	// the token is in that item. Off by default and never exported.
	TouchID bool `json:"touchId"`
}
