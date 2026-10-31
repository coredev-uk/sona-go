//go:build cgo

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const deviceHelp = "not supported on macOS: the output follows the system default (System Settings → Sound); any value fails"

// audioStream says where the audio goes: the child's osxaudiosink plays
// to the system's default output.
func audioStream(int) {
	step("output", "device", "the system default (osxaudiosink)")
}

// memory prints the resident set size of this process and the host (macOS
// has no proportional set size).
func memory(label string, child int) {
	parent := rss(os.Getpid())
	host := rss(child)
	step(label, "parent_rss", fmtKiB(parent), "host_rss", fmtKiB(host), "total", fmtKiB(parent+host))
}

func fmtKiB(k int) string { return fmt.Sprintf("%.1fMiB", float64(k)/1024) }

// rss is ps's resident set size of pid in KiB, 0 if unknown.
func rss(pid int) int {
	out, err := exec.Command("/bin/ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
}
