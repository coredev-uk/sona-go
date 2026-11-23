//go:build cgo

package gst

/*
#cgo LDFLAGS: -lole32
#include <stdlib.h>
#include "devices.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// routeEpoch moves when an endpoint is added or removed or changes state
// (plugged in, unplugged, enabled, disabled): the chosen device may have
// gone or come back, which moves the output (streamProps). A change of the
// default needs no move: wasapi2sink on the default is moved by Windows
// itself (automatic stream routing).
func routeEpoch() uint64 { return uint64(C.sona_route_epoch()) }

// AudioSinks lists the active WASAPI render endpoints, as wasapi2's device
// provider does, from Core Audio directly (devices_windows.c). A Device's
// Name is the endpoint ID wasapi2sink takes as its device. It neither
// initialises GStreamer nor loads a plugin: it runs in the UI process, and a
// plugin loaded from the install's lib\ keeps an update from renaming it.
func AudioSinks() ([]Device, error) {
	var list *C.sona_device
	var n C.int
	if C.sona_gst_audio_sinks(&list, &n) != 0 {
		return nil, errors.New("gst: listing the WASAPI render endpoints failed")
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

// endpoint is what Windows says of an endpoint ID (sona_endpoint_state).
type endpoint int

const (
	endpointUnknown endpoint = C.SONA_ENDPOINT_UNKNOWN // the endpoint API failed
	endpointGone    endpoint = C.SONA_ENDPOINT_GONE    // unknown, unplugged or disabled
	endpointActive  endpoint = C.SONA_ENDPOINT_ACTIVE  // can play
)

func endpointState(id string) endpoint {
	c := C.CString(id)
	defer C.free(unsafe.Pointer(c))
	return endpoint(C.sona_endpoint_state(c))
}

// streamProps returns wasapi2sink's device: the endpoint ID of the device to
// play to now, "" for the default (outputTarget). Other sinks play to the
// default (wasapisink takes another kind of device ID).
func streamProps(sink, _, device string) string {
	return outputTarget(sink, device, endpointState)
}

// outputTarget is wasapi2sink's device for the chosen device: device while
// it can play, else "" (the default, which Windows moves the stream to when
// it changes) until it is back. When Windows cannot say (state fails),
// device is kept: the sink reports whether it opens.
func outputTarget(sink, device string, state func(id string) endpoint) string {
	if sink != "wasapi2sink" || device == "" || state(device) == endpointGone {
		return ""
	}
	return device
}
