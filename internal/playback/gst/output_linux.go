package gst

/*
#include "devices.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

// routeEpoch never moves: PipeWire moves a stream that follows the default
// itself, and links one whose target comes back (SONA_MSG_RELINK covers the
// relink).
func routeEpoch() uint64 { return 0 }

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

// Sinks are the sinks tried in order when Options leaves Sink empty.
var Sinks = []string{"pipewiresink", "pulsesink", "autoaudiosink"}

// streamProps returns the sink's stream-properties: PipeWire wants its own
// property names and capitalised roles, PulseAudio lower-case roles.
func streamProps(sink, app, device string) string {
	clean := strings.NewReplacer(`"`, ``, `\`, ``)
	esc := clean.Replace(app)
	switch sink {
	case "pipewiresink":
		props := fmt.Sprintf(`props,media.role=(string)Music,media.category=(string)Playback,media.name=(string)"%s",application.name=(string)"%s",application.id=(string)sona,node.description=(string)"%s"`, esc, esc, esc)
		if device != "" {
			// What pipewiresink's target-object sets: the session manager
			// links the stream to this node (or the default if it is gone).
			props += fmt.Sprintf(`,target.object=(string)"%s"`, clean.Replace(device))
		}
		return props
	case "pulsesink":
		return fmt.Sprintf(`props,media.role=(string)music,application.name=(string)"%s",application.id=(string)sona`, esc)
	}
	return ""
}
