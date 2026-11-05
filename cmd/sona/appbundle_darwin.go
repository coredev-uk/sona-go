//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// runsFromAppBundle reports whether this process is Sona.app's executable.
// macOS ties notifications (UNUserNotificationCenter) and the login item
// (SMAppService) to an app bundle: from a bare binary, MyGo's
// NotificationsSupported is false and SetOpenAtLogin fails, so the platform
// offers neither (platform_darwin.go) and says it is the bundle they need.
func runsFromAppBundle() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return inAppBundle(exe)
}

// inAppBundle reports whether exe is <name>.app/Contents/MacOS/<exe>, where
// MyGo (NSBundle.mainBundle) finds the bundle.
func inAppBundle(exe string) bool {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	return filepath.Base(macos) == "MacOS" && filepath.Base(contents) == "Contents" &&
		strings.HasSuffix(filepath.Dir(contents), ".app")
}
