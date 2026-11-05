//go:build !darwin

package app

// systemSwipeSetting is nil off macOS: the swipe is off.
func systemSwipeSetting() func() bool { return nil }
