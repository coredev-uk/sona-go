package player

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// control is one of the snapshot's Controls the bar and its shortcuts ask
// about (v0.2.0's availability.ts).
type control int

const (
	ctlPrevious control = iota
	ctlNext
	ctlSeek
	ctlShuffle
	ctlRepeat
	ctlAutoPlay
	ctlQueue
)

// applies reports whether Go says the control applies to what plays.
func applies(c services.PlaybackControls, ctl control) bool {
	switch ctl {
	case ctlPrevious:
		return c.Previous
	case ctlNext:
		return c.Next
	case ctlSeek:
		return c.Seek
	case ctlShuffle:
		return c.Shuffle
	case ctlRepeat:
		return c.Repeat
	case ctlAutoPlay:
		return c.AutoPlay
	case ctlQueue:
		return c.Queue
	}
	return true
}

// unavailable is why ctl doesn't apply to what plays, or "" when it does
// (kit.ControlUnavailable: Go decides, Controls). Next at the end of a queue
// is simply off.
func unavailable(s services.PlaybackState, ctl control) string {
	return kit.ControlUnavailable(s, applies(s.Controls, ctl))
}

// refuse stops a command (a key, a menu entry) that doesn't apply: it says
// why in the notifications region (one toast at a time, however fast the key
// repeats) and returns true.
func (p *player) refuse(ctl control) bool {
	reason := unavailable(p.env.Play.State(), ctl)
	if reason == "" {
		return false
	}
	kit.Unavailable(reason)
	return true
}
