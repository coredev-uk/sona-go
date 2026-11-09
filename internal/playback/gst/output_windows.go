//go:build cgo

package gst

/*
#include "devices.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// routeEpoch never moves: wasapi2sink on the default device is moved by
// Windows itself (automatic stream routing), and one on a chosen device that
// goes away fails, which rebuilds the output.
func routeEpoch() uint64 { return 0 }

// AudioSinks lists the WASAPI render endpoints (wasapi2's device provider). A
// Device's Name is the endpoint ID wasapi2sink takes as its device. It needs
// no running pipeline. The list is empty when the wasapi2 plugin is missing.
func AudioSinks() ([]Device, error) {
	if err := Init(); err != nil {
		return nil, err
	}
	var list *C.sona_device
	var n C.int
	if C.sona_gst_audio_sinks(&list, &n) != 0 {
		return nil, errors.New("gst: WASAPI device provider failed")
	}
	defer C.sona_gst_free_devices(list, n)
	out := make([]Device, 0, int(n))
	for _, d := range unsafe.Slice(list, int(n)) {
		out = append(out, Device{Name: C.GoString(d.name), Description: C.GoString(d.description), Default: d.is_default != 0})
	}
	return out, nil
}

// Sinks are the sinks tried in order when Options leaves Sink empty:
// WASAPI through the newer element (Windows 10 and later, which follows the
// default device), the original one, then DirectSound and whatever
// GStreamer picks.
var Sinks = []string{"wasapi2sink", "wasapisink", "directsoundsink", "autoaudiosink"}

// streamProps returns wasapi2sink's device: the endpoint ID of the device to
// play to, "" for the default. Other sinks play to the default (wasapisink
// takes another kind of device ID).
func streamProps(sink, _, device string) string {
	if sink == "wasapi2sink" {
		return device
	}
	return ""
}
