//go:build darwin

package services

import "os/exec"

// passkeySupported: macOS has Safari, which offers the account's passkeys
// (iCloud Keychain) with Touch ID, as music.apple.com does.
const passkeySupported = true

// openInPasskeyBrowser opens url in Safari, whichever browser is the
// default: Safari is the one that is signed in to iCloud Keychain with Touch
// ID for sure.
func openInPasskeyBrowser(url string) error {
	return exec.Command("/usr/bin/open", "-b", "com.apple.Safari", url).Run()
}
