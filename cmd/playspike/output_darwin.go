//go:build cgo

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

const deviceHelp = "Core Audio output device UID to play to (default: the system default; one that isn't connected plays there too)"

// audioStream lists Core Audio's outputs and marks the one -device chose
// and the system default; the child's osxaudiosink plays to the chosen one
// while it is connected, else to the default.
func audioStream(int) {
	devs, err := gst.AudioSinks()
	if err != nil {
		step("output", "err", err)
		return
	}
	for _, d := range devs {
		step("output", "uid", d.Name, "name", d.Description, "default", d.Default, "chosen", d.Name == outputDevice)
	}
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
