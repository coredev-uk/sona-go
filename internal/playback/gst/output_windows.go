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
// (plugged in, unplugged, enabled, disabled), or the default endpoint
// changes: the device to play to may differ (streamProps), which moves the
// output.
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

// defaultEndpoint is the default console render endpoint's ID, "" without
// one or when Windows cannot say.
func defaultEndpoint() string {
	c := C.sona_default_endpoint()
	if c == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(c))
	return C.GoString(c)
}

// streamProps returns wasapi2sink's device: the endpoint ID of the device to
// play to now (outputTarget). Other sinks play to the default (wasapisink
// takes another kind of device ID).
func streamProps(sink, _, device string) string {
	return outputTarget(sink, device, endpointState, defaultEndpoint)
}

// outputTarget is wasapi2sink's device for the chosen device: device while
// it can play, else the default endpoint's ID (def) until it is back. When
// Windows cannot say (state fails), device is kept: the sink reports whether
// it opens. The default is named rather than left to wasapi2sink ("") since
// its stream stays on the endpoint it opened when the default changes
// (#312): the output is rebuilt when def changes (routeEpoch). Without a
// default ID, "" lets the sink find one itself.
func outputTarget(sink, device string, state func(id string) endpoint, def func() string) string {
	if sink != "wasapi2sink" {
		return ""
	}
	if device != "" && state(device) != endpointGone {
		return device
	}
	return def()
}
