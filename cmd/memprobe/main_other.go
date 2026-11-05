//go:build !linux

// Command memprobe measures a running probe build's memory from /proc, so it
// runs on Linux only; on macOS Settings → Diagnostics → Copy diagnostics has
// a Memory section with the app's and the playback host's footprint.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "memprobe reads /proc and runs on Linux only")
	os.Exit(2)
}
