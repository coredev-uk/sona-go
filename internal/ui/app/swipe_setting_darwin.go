package app

import (
	"context"
	"os/exec"
	"time"
)

// systemSwipeSetting reads whether macOS turns a two-finger sideways scroll
// into a swipe between pages.
func systemSwipeSetting() func() bool {
	return func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleEnableSwipeNavigateWithScrolls").Output()
		return parseSwipeSetting(string(out), err)
	}
}
