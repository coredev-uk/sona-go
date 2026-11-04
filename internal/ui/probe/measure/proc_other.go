//go:build !linux

package measure

import (
	"time"
)

// Usage is what a process and its children have used so far; only Linux
// reports it (the probes run in the headless Linux session).
type Usage struct {
	CPU      time.Duration
	RSS, PSS int
	Procs    int
	OK       bool
}

// TreeUsage is not available off Linux.
func TreeUsage(int) Usage { return Usage{} }
