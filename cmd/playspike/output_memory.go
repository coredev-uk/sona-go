//go:build linux || (cgo && darwin)

package main

import (
	"fmt"
	"os"

	"github.com/coredev-uk/sona-go/internal/procmem"
)

// memory prints the memory footprint of this process and the host
// (procmem.Usage.Footprint: the proportional set size on Linux, the
// physical footprint on macOS).
func memory(label string, child int) {
	parent := footprint(os.Getpid())
	host := footprint(child)
	step(label, "parent_footprint", fmtBytes(parent), "host_footprint", fmtBytes(host), "total", fmtBytes(parent+host))
}

// footprint is pid's footprint in bytes, 0 if unknown.
func footprint(pid int) uint64 {
	u, err := procmem.Of(pid)
	if err != nil {
		return 0
	}
	return u.Footprint
}

func fmtBytes(b uint64) string { return fmt.Sprintf("%.1fMiB", float64(b)/(1<<20)) }
