//go:build !darwin

package store

// SystemBiometric is nil off macOS: no biometric Keychain exists there.
func SystemBiometric(prompt func() string) BiometricKeychain { return nil }
