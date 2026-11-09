//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
)

// Timing of music video pictures against the audio clock: the audio
// being heard is the master, and each decoded picture is sent when the
// audio reaches its presentation time.

const (
	// frameLead sends a picture this much before the audio reaches it,
	// for the time it takes to reach the screen: the pipe to the parent,
	// the web view's fetch and its next paint (about one 60 Hz frame).
	frameLead = 20 * time.Millisecond
	// staleBefore: with only one picture queued, it is known to come
	// before the start (a seek's target) once it is this far before it,
	// longer than any picture lasts.
	staleBefore = 200 * time.Millisecond
	// presentPoll bounds how long the presenter sleeps before looking at
	// the clock again (a pause, a seek, the decoder catching up).
	presentPoll = 40 * time.Millisecond
	// presentMin is the shortest sleep, so a picture due now is not
	// waited for in a busy loop.
	presentMin = time.Millisecond
)

// framePlan is what to do with the queued pictures now.
type framePlan struct {
	drop int  // leading pictures to discard (before the start, or late)
	show bool // then send the picture at the head of the queue
	// wait is how long to sleep before planning again.
	wait time.Duration
}

// planFrames decides what to do with the queued pictures, whose
// presentation times are pts (in presentation order). start is the first
// position to show (a load's or seek's target); pos the audio position
// heard now, when known (havePos). shown says whether a picture was sent
// since the load.
//
// Pictures superseded before start are dropped. Until one is shown, the
// first picture at start is sent at once, so a paused seek shows where it
// landed. After that the newest picture due (pts <= pos + frameLead) is
// sent and the earlier ones are dropped as late; with none due, wait is
// the time until the first is.
func planFrames(pts []time.Duration, start, pos time.Duration, havePos, shown bool) framePlan {
	p := framePlan{wait: presentPoll}
	n := len(pts)
	for p.drop < n {
		i := p.drop
		if i+1 < n && pts[i+1] <= start {
			p.drop++ // the next picture is still at or before start
			continue
		}
		if i+1 == n && pts[i] < start-staleBefore {
			p.drop++ // alone, and too early to be the one at start
			continue
		}
		break
	}
	rest := pts[p.drop:]
	if len(rest) == 0 {
		return p
	}
	if !shown {
		p.show = true
		p.wait = presentMin
		return p
	}
	if !havePos {
		return p
	}
	due := -1
	for i, t := range rest {
		if t > pos+frameLead {
			break
		}
		due = i
	}
	if due < 0 {
		p.wait = min(max(rest[0]-frameLead-pos, presentMin), presentPoll)
		return p
	}
	p.drop += due
	p.show = true
	p.wait = presentMin
	return p
}

// videoTime is a video sample's presentation time: its composition time
// less the video's edit list start (VideoMedia.Shift then puts it on the
// track's timeline).
func videoTime(s *fmp4.Sample, in *fmp4.Init) time.Duration {
	pts := s.PTS()
	if in.HasEdit {
		pts -= in.EditStart
	}
	if pts < 0 {
		return -fmp4.Time(-pts, in.Timescale)
	}
	return fmp4.Time(pts, in.Timescale)
}
