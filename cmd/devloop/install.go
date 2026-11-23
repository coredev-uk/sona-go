package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// install puts the new build (next) in place of out. Windows won't replace or
// delete an executable a process still runs: the app's playback host child,
// the same binary, can outlive a killed app by a moment, and the restart
// failed with "Access is denied". It does let that file be renamed, so there
// the old build first moves aside (out.old-<time>); the ones left from earlier
// restarts are removed once nothing runs them.
func install(next, out string) error {
	if runtime.GOOS == "windows" {
		removeAside(out)
		if _, err := os.Stat(out); err == nil {
			if err := os.Rename(out, fmt.Sprintf("%s.old-%d", out, time.Now().UnixNano())); err != nil {
				return err
			}
		}
	}
	return os.Rename(next, out)
}

// removeAside deletes the old builds install moved aside; one still running
// stays for the next time.
func removeAside(out string) {
	old, _ := filepath.Glob(out + ".old-*")
	for _, f := range old {
		_ = os.Remove(f)
	}
}
