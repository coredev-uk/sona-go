package coordinator

import (
	"fmt"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// sleepStep is how often the sleep fade lowers the volume.
const sleepStep = 250 * time.Millisecond

// SetSleep sets the sleep timer: SleepAfter pauses playback d from now,
// SleepTrack when the current item ends and SleepQueue when the last of
// the user's items ends (the next item is then selected, paused, so
// Resume continues). Playback fades out over the last SleepFade and the
// volume is restored once it stops. SleepOff cancels the timer.
func (c *Coordinator) SetSleep(mode SleepMode, d time.Duration) error {
	switch mode {
	case SleepOff, SleepTrack, SleepQueue:
	case SleepAfter:
		if d <= 0 {
			return fmt.Errorf("coordinator: sleep duration %s must be positive", d)
		}
	default:
		return fmt.Errorf("coordinator: invalid sleep mode %q", mode)
	}
	c.mu.Lock()
	c.sleep = Sleep{Mode: mode}
	if mode == SleepAfter {
		c.sleep.At = c.now().Add(d)
	}
	// Preload and the engine's armed next depend on where playback stops.
	c.schedulePreloadLocked()
	c.mu.Unlock()
	c.notify()
	return nil
}

// sleepEndLocked returns when the sleep timer stops playback, or false
// when that time is unknown (SleepTrack and SleepQueue only know it while
// the item that ends playback is playing).
func (c *Coordinator) sleepEndLocked(now time.Time) (time.Time, bool) {
	switch c.sleep.Mode {
	case SleepAfter:
		return c.sleep.At, true
	case SleepTrack, SleepQueue:
		if !c.sleepEndsHereLocked() || !c.loaded || c.st.State != playback.StatePlaying || c.st.Duration <= 0 {
			return time.Time{}, false
		}
		return now.Add(c.st.Duration - c.positionLocked(now)), true
	}
	return time.Time{}, false
}

// sleepEndsHereLocked reports whether the sleep timer stops playback when
// the current item ends.
func (c *Coordinator) sleepEndsHereLocked() bool {
	if c.index < 0 {
		return false
	}
	switch c.sleep.Mode {
	case SleepTrack:
		return true
	case SleepQueue:
		for _, e := range c.queue[c.index+1:] {
			if !e.auto && !c.withheldLocked(e.item) {
				return false
			}
		}
		return true
	}
	return false
}

// sleepVolumeLocked is the engine volume the fade wants now, and whether
// the fade is under way.
func (c *Coordinator) sleepVolumeLocked(now time.Time) (float64, bool) {
	end, ok := c.sleepEndLocked(now)
	if !ok || c.index < 0 || !c.loaded || c.st.State != playback.StatePlaying {
		return c.volume, false
	}
	left := end.Sub(now)
	if left > SleepFade {
		return c.volume, false
	}
	return c.volume * float64(max(left, 0)) / float64(SleepFade), true
}

// scheduleSleepLocked (re)arms the sleep timer's next tick: the fade's
// start, the next fade step (aligned to the end, so frequent reschedules
// cannot starve it), the SleepAfter deadline, or a volume restore.
func (c *Coordinator) scheduleSleepLocked() {
	if c.sleepTimer != nil {
		c.sleepTimer.Stop()
		c.sleepTimer = nil
	}
	if c.closed {
		return
	}
	now := c.now()
	delay := time.Duration(-1)
	if end, ok := c.sleepEndLocked(now); ok {
		_, fading := c.sleepVolumeLocked(now)
		left := end.Sub(now)
		switch {
		case left <= 0 && c.sleep.Mode == SleepAfter:
			delay = 0
		case left <= 0: // the item is about to end; its end event stops playback
			delay = sleepStep
		case fading:
			delay = left % sleepStep
			if delay == 0 {
				delay = sleepStep
			}
		case c.st.State == playback.StatePlaying:
			delay = max(left-SleepFade, 0)
		default: // SleepAfter while not playing: only the deadline matters
			delay = left
		}
	}
	if c.faded {
		if _, fading := c.sleepVolumeLocked(now); !fading {
			delay = 0 // restore the volume
		}
	}
	if delay >= 0 {
		c.sleepTimer = time.AfterFunc(delay, c.sleepTick)
	}
}

// sleepTick applies the fade, pauses at the SleepAfter deadline and
// restores the volume.
func (c *Coordinator) sleepTick() {
	c.cmd.Lock()
	defer c.cmd.Unlock()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	now := c.now()
	if c.sleep.Mode == SleepAfter && !now.Before(c.sleep.At) {
		c.sleep = Sleep{}
		pause := c.index >= 0 && c.loaded && (c.st.State == playback.StatePlaying || c.st.State == playback.StateLoading)
		var uid uint64
		if pause {
			uid = c.queue[c.index].uid
		}
		restore, vol := c.faded, c.volume
		c.faded = false
		c.schedulePreloadLocked()
		c.mu.Unlock()
		if pause {
			if err := c.eng.Pause(c.ctx); err != nil {
				c.log.Warn("coordinator: sleep timer pause", "err", err)
			}
		}
		c.restoreVolume(restore, vol)
		if pause {
			c.refresh(uid)
		} else {
			c.notify()
		}
		return
	}
	want, fading := c.sleepVolumeLocked(now)
	apply := fading || c.faded
	c.faded = fading
	c.scheduleSleepLocked()
	c.mu.Unlock()
	if apply {
		if err := c.eng.SetVolume(c.ctx, want); err != nil {
			c.log.Warn("coordinator: sleep fade", "err", err)
		}
	}
}

// sleepStopLocked ends playback for the sleep timer after the current
// item ended naturally (fx is its finalize call): the timer clears, the
// next item is selected paused and the volume comes back. Called with cmd
// and mu held; returns with mu released.
func (c *Coordinator) sleepStopLocked(fx func()) {
	c.sleep = Sleep{}
	restore, vol := c.faded, c.volume
	c.faded = false
	if next, ok := c.nextIndexLocked(false); ok {
		c.selectLocked(next, 0, true, playback.StatePaused)
	} else {
		c.schedulePreloadLocked()
	}
	c.mu.Unlock()
	run(fx)
	c.restoreVolume(restore, vol)
	c.notify()
}

// restoreVolume sets the engine back to the user's volume after a fade.
// Caller holds cmd, not mu.
func (c *Coordinator) restoreVolume(faded bool, vol float64) {
	if !faded {
		return
	}
	if err := c.eng.SetVolume(c.ctx, vol); err != nil {
		c.log.Warn("coordinator: restore volume after sleep", "err", err)
	}
}
