package native

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// SetEqualizer sets the equaliser of this and every later host; a running
// host applies it to the playing audio at once.
func (e *Engine) SetEqualizer(ctx context.Context, eq cdmhost.Equalizer) error {
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
// audio without interrupting it.
func (e *Engine) SetOutputDevice(ctx context.Context, device string) error {
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
