//go:build cgo

package gst

import (
	"errors"
	"fmt"
)

// DeviceChoice: on macOS the output always follows the system default
// (System Settings → Sound). osxaudiosink can play to another device, but
// Sona doesn't choose one there yet.
const DeviceChoice = false

// ErrNoDeviceChoice reports that this build always plays to the system
// default output. It wraps errors.ErrUnsupported.
var ErrNoDeviceChoice = fmt.Errorf("gst: choosing the output device isn't supported on macOS: %w", errors.ErrUnsupported)

// AudioSinks fails with ErrNoDeviceChoice.
func AudioSinks() ([]Device, error) { return nil, ErrNoDeviceChoice }

// CanPlayTo accepts only "" (the system default).
func CanPlayTo(device string) error {
	if device != "" {
		return fmt.Errorf("%w (asked for %q)", ErrNoDeviceChoice, device)
	}
	return nil
}

// Sinks are the sinks tried in order when Options leaves Sink empty:
// Core Audio's, then whatever GStreamer picks.
var Sinks = []string{"osxaudiosink", "autoaudiosink"}

// streamProps is empty: osxaudiosink has no stream properties.
func streamProps(string, string, string) string { return "" }
