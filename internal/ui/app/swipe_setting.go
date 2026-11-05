package app

import (
	"strings"
)

// parseSwipeSetting reads `defaults read -g AppleEnableSwipeNavigateWithScrolls`
// (macOS's System Settings → Trackpad → More Gestures → "Swipe between
// pages": "Scroll left or right with two fingers" and "Swipe with two or
// three fingers" set it, "Swipe with three fingers" and off clear it): the
// default is absent, which is on. NSEvent.isSwipeTrackingFromScrollEventsEnabled
// is the same setting, which MyGo does not expose.
func parseSwipeSetting(out string, err error) bool {
	if err != nil {
		return true // the key is not set
	}
	switch strings.TrimSpace(out) {
	case "0", "false", "NO":
		return false
	}
	return true
}

// refreshSwipe reads the system's setting again, off the main thread (it runs
// a process), and applies it there. It is at start and each time the window
// takes the focus, when it may have been changed. Nothing where the system
// has none (swipeSetting is nil).
func (a *App) refreshSwipe() {
	read := a.swipeSetting
	if read == nil {
		return
	}
	post := a.postFunc()
	go func() {
		on := read()
		post(func() { a.swipe.Enabled = on })
	}()
}
