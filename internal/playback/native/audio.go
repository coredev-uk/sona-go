//go:build linux || (darwin && cgo)

package native

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// Capabilities reports everything the CDM host does itself: volume, the
// equaliser, Sound Check, crossfades (not between previews, which join
// like uploads) and, where its sink can target one (PipeWire;
// gst.DeviceChoice), the output device. Sona sends every play report.
func (e *Engine) Capabilities() playback.Capabilities {
	return playback.Capabilities{
		Kind:             playback.KindCDM,
		Volume:           true,
		Equalizer:        true,
		SoundCheck:       true,
		Crossfade:        !e.preview,
		OutputDevice:     gst.DeviceChoice,
		PrivateListening: true,
	}
}

// OutputDevices lists the audio outputs SetOutputDevice can target
// (PipeWire sinks). The list is empty when PipeWire's GStreamer plugin is
// missing; playback then always follows the default output. On macOS it
// fails with an error wrapping errors.ErrUnsupported.
func (e *Engine) OutputDevices() ([]playback.OutputDevice, error) {
	devs, err := gst.AudioSinks()
	if err != nil {
		return []playback.OutputDevice{}, err
	}
	out := make([]playback.OutputDevice, len(devs))
	for i, d := range devs {
		out[i] = playback.OutputDevice{Name: d.Name, Description: d.Description, Default: d.Default}
	}
	return out, nil
}

// SetEqualizer sets the equaliser of this and every later host; a running
// host applies it to the playing audio at once.
func (e *Engine) SetEqualizer(ctx context.Context, eq equalizer.Curve) error {
	e.mu.Lock()
	e.eq = eq
	h := e.h
	e.mu.Unlock()
	if h == nil {
		return nil
	}
	return h.SetEqualizer(ctx, eq)
}

// SetOutputDevice plays to the PipeWire sink device (its node.name) from
// now on; "" follows the system default. A running host moves the playing
// audio without interrupting it. Where the sink can't choose (macOS,
// gst.CanPlayTo) any device but "" fails with an error wrapping
// errors.ErrUnsupported and changes nothing.
func (e *Engine) SetOutputDevice(ctx context.Context, device string) error {
	if err := gst.CanPlayTo(device); err != nil {
		return err
	}
	e.mu.Lock()
	e.device = device
	h := e.h
	e.mu.Unlock()
	if h == nil {
		return nil
	}
	return h.SetOutputDevice(ctx, device)
}

// SetSoundCheck turns Sound Check on or off: each item plays with the gain
// that brings its measured loudness to soundcheck.Target. A running host
// applies it to the playing item at once.
func (e *Engine) SetSoundCheck(ctx context.Context, on bool) error {
	e.mu.Lock()
	e.soundCheck = on
	h := e.h
	e.mu.Unlock()
	if h == nil {
		return nil
	}
	return h.SetTrackGain(ctx, on)
}
