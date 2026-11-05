//go:build !darwin

package services

import "errors"

// passkeySupported is false off macOS: there is no browser Sona can rely on
// for the account's Touch ID passkey.
const passkeySupported = false

func openInPasskeyBrowser(string) error { return errors.ErrUnsupported }
