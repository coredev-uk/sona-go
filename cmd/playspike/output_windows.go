//go:build cgo

package main

import "github.com/coredev-uk/sona-go/internal/playback/gst"

const deviceHelp = "WASAPI endpoint ID to play to (default: the system default output)"

// audioStream reports the output: the sink the child chose is in the
// "audio" line already, and the endpoints are listed here (none yet:
// gst.AudioSinks).
func audioStream(int) {
	devs, err := gst.AudioSinks()
	if err != nil {
		step("output", "err", err)
		return
	}
	for _, d := range devs {
		step("output", "id", d.Name, "name", d.Description, "default", d.Default, "chosen", d.Name == outputDevice)
	}
}

// memory says memory is not measured on Windows.
func memory(label string, _ int) {
	step(label, "measured", false)
}
