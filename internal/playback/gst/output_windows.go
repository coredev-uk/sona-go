//go:build cgo

package gst

// routeEpoch never moves: a WASAPI stream that follows the default device
// is moved by Windows itself.
func routeEpoch() uint64 { return 0 }

// AudioSinks lists nothing yet: the output follows the default device.
func AudioSinks() ([]Device, error) {
	if err := Init(); err != nil {
		return nil, err
	}
	return nil, nil
}

// Sinks are the sinks tried in order when Options leaves Sink empty:
// WASAPI through the newer element (Windows 10 and later), the original
// one, then DirectSound and whatever GStreamer picks.
var Sinks = []string{"wasapi2sink", "wasapisink", "directsoundsink", "autoaudiosink"}

// streamProps returns the WASAPI sinks' device property: the endpoint ID of
// the device to play to, "" for the default. Other sinks get nothing.
func streamProps(sink, _, device string) string {
	switch sink {
	case "wasapi2sink", "wasapisink":
		return device
	}
	return ""
}
