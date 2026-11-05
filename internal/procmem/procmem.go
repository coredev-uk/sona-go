// Package procmem reads how much memory a process holds, as the operating
// system counts it, for the diagnostics bundle (Settings → Diagnostics): the
// app's own process and the playback host child show up as two processes in
// Activity Monitor and in `top`, and a report of "400 MB" needs to say which.
package procmem

import "errors"

// Usage is a process's memory in bytes.
type Usage struct {
	// Resident is the pages in RAM (RSS): it counts every shared library
	// page, and on macOS what the compressor holds apart.
	Resident uint64 `json:"resident"`
	// Footprint is the figure the platform's own tools show for the
	// process: on macOS the physical footprint (Activity Monitor's Memory
	// column: private dirty and compressed memory, GPU buffers the process
	// owns, no clean file pages), on Linux the proportional set size (shared
	// pages split among the processes that map them).
	Footprint uint64 `json:"footprint"`
}

// ErrUnsupported is returned where the platform has no reader.
var ErrUnsupported = errors.ErrUnsupported
