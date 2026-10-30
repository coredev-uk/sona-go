package gst

/*
#cgo LDFLAGS: -lm
#include "devices.h"
*/
import "C"

import (
	"errors"
	"unsafe"
)

// Device is an audio output.
type Device struct {
	Name        string // PipeWire node.name, the sink's target-object
	Description string // human-readable name
	Default     bool   // the system's default output
}

// AudioSinks lists PipeWire's audio sinks. It needs no running pipeline
// and touches nothing but the local PipeWire daemon. The list is empty when
// PipeWire's GStreamer plugin is not installed (other sinks have no target
// to choose).
func AudioSinks() ([]Device, error) {
	if err := Init(); err != nil {
		return nil, err
	}
	var list *C.sona_device
	var n C.int
	if C.sona_gst_audio_sinks(&list, &n) != 0 {
		return nil, errors.New("gst: PipeWire device provider failed")
	}
	defer C.sona_gst_free_devices(list, n)
	out := make([]Device, 0, int(n))
	for _, d := range unsafe.Slice(list, int(n)) {
		out = append(out, Device{Name: C.GoString(d.name), Description: C.GoString(d.description), Default: d.is_default != 0})
	}
	return out, nil
}
