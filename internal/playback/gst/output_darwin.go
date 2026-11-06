//go:build cgo

package gst

/*
#cgo LDFLAGS: -framework CoreAudio -framework CoreFoundation
#include <stdlib.h>
#include "devices.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// AudioSinks lists Core Audio's output devices. A Device's Name is its
// UID (osxaudiosink's unique-id), which stays the same across reconnections
// and restarts. It needs no running pipeline and no GStreamer.
func AudioSinks() ([]Device, error) {
	var list *C.sona_device
	var n C.int
	if C.sona_gst_audio_sinks(&list, &n) != 0 {
		return nil, errors.New("gst: listing Core Audio's output devices failed")
	}
	defer C.sona_gst_free_devices(list, n)
	out := make([]Device, 0, int(n))
	for _, d := range unsafe.Slice(list, int(n)) {
		out = append(out, Device{Name: C.GoString(d.name), Description: C.GoString(d.description), Default: d.is_default != 0})
	}
	return out, nil
}

// Sinks are the sinks tried in order when Options leaves Sink empty:
// Core Audio's, then whatever GStreamer picks.
var Sinks = []string{"osxaudiosink", "autoaudiosink"}

// streamProps returns osxaudiosink's unique-id: the UID of the device to
// play to now, device while it can play, else the system default output.
// osxaudiosink opens the device it is given and stays on it, so the output
// is rebuilt when this changes (routeEpoch). Other sinks get nothing.
func streamProps(sink, _, device string) string {
	if sink != "osxaudiosink" {
		return ""
	}
	var c *C.char
	if device != "" {
		c = C.CString(device)
		defer C.free(unsafe.Pointer(c))
	}
	t := C.sona_output_target(c)
	if t == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(t))
	return C.GoString(t)
}

// routeEpoch moves when the system default output changes or a device is
// plugged in or removed: the device streamProps resolves to may differ.
func routeEpoch() uint64 { return uint64(C.sona_route_epoch()) }
