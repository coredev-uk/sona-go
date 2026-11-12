//go:build cgo

package main

import "github.com/coredev-uk/sona-go/internal/playback/gst"

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
