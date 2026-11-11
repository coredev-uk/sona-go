package coordinator

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// ErrRestricted: nothing left to play, because the account restricts
// explicit content and every remaining item is explicit.
var ErrRestricted = errors.New("coordinator: explicit content is restricted on this account")

// replaced is the queue as it was before SetQueue or PlayStation
// replaced it, for UndoReplace.
type replaced struct {
	queue    []entry
	original []uint64
	shuffle  bool
	index    int
	pos      time.Duration
	playing  bool
	label    string
	station  *Station
}

// replacedLocked captures the queue about to be replaced (nil when it is
// empty).
func (c *Coordinator) replacedLocked() *replaced {
	if len(c.queue) == 0 {
		return nil
	}
	r := &replaced{
		queue:    slices.Clone(c.queue),
		original: slices.Clone(c.original),
		shuffle:  c.shuffle,
		index:    c.index,
		pos:      c.positionLocked(c.now()),
		playing:  c.loaded && (c.st.State == playback.StatePlaying || c.st.State == playback.StateLoading),
		label:    c.label,
	}
	if c.station != nil {
		st := *c.station
		r.station = &st
	}
	return r
}

// UndoReplace brings back the queue the last SetQueue or PlayStation
// replaced: its items, shuffle order, station and current item at the
// position it had, playing if it was playing and paused otherwise. It
// reports whether there was a queue to bring back; Stop forgets it.
func (c *Coordinator) UndoReplace(ctx context.Context) (bool, error) {
	ctx, leave := c.enter(ctx, func() bool { return c.undo != nil })
	defer leave()
	c.mu.Lock()
	u := c.undo
	if u == nil {
		c.mu.Unlock()
		return false, nil
	}
	c.undo = nil
	fx := c.finalizeLocked(false, EndSelected)
	loaded := c.loaded
	c.queue, c.original, c.shuffle, c.label = u.queue, u.original, u.shuffle, u.label
	c.dropStationLocked()
	c.station = u.station
	c.resetLocked()
	c.queueChangedLocked()
	if u.index >= 0 && u.playing {
		c.mu.Unlock()
		run(fx)
		return true, c.start(ctx, u.index, u.pos, true)
	}
	if u.index >= 0 {
		c.selectLocked(u.index, u.pos, true, playback.StatePaused)
	}
	c.mu.Unlock()
	c.notify()
	run(fx)
	return true, c.stopEngine(ctx, loaded)
}

// SetRestricted sets whether the account restricts explicit content:
// while it does, explicit items are skipped (an explicit item already
// playing finishes).
func (c *Coordinator) SetRestricted(on bool) {
	c.mu.Lock()
	if c.restricted == on {
		c.mu.Unlock()
		return
	}
	c.restricted = on
	c.schedulePreloadLocked()
	c.mu.Unlock()
	c.notify()
}
