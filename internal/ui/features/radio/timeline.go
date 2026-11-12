package radio

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// This file is the programme guide's arithmetic: which stretch of time the
// board shows, where a programme sits in it, and where its text goes. It is
// numbers in, numbers out; the board and the station page draw them.

// programme is a stretch of one station's air time under one title: a slot,
// or the unbroken run of back-to-back slots with the same title (a station's
// own hours, a show's consecutive hours), which the guide counts as one.
type programme struct {
	span
	// event is the run's first slot: its title, description and art.
	event services.StationEvent
}

// programmes are a schedule's slots with the back-to-back ones of one title
// joined, in the schedule's order.
func programmes(schedule []services.StationEvent) []programme {
	out := make([]programme, 0, len(schedule))
	for _, e := range schedule {
		if n := len(out); n > 0 && out[n-1].endMs == e.StartMs && same(out[n-1].event.Title, e.Title) {
			out[n-1].endMs = e.EndMs
			continue
		}
		out = append(out, programme{span: spanOf(e), event: e})
	}
	return out
}

// onAir is true while now is inside the programme.
func (p programme) onAir(now int64) bool { return p.startMs <= now && now < p.endMs }

// The board's time scale: how much of the day its width shows. The window
// starts leadTime before now, so the programme that just ended is still in
// view, and is as long as the width allows at about pxPerMinute.
const (
	leadTime    = 30 * time.Minute
	pxPerMinute = 3.4
	minWindow   = 90 * time.Minute
	maxWindow   = 300 * time.Minute
)

// window is the stretch of time the board's track shows.
type window struct{ start, end int64 }

// windowAt is the window for a track trackW DIPs wide at now (Unix ms): from
// leadTime before the minute now falls in, as many half hours long as fit at
// pxPerMinute, between minWindow and maxWindow.
func windowAt(now int64, trackW float32) window {
	minutes := int(trackW/pxPerMinute) / 30 * 30
	length := min(max(time.Duration(minutes)*time.Minute, minWindow), maxWindow)
	start := now - now%time.Minute.Milliseconds() - leadTime.Milliseconds()
	return window{start, start + length.Milliseconds()}
}

// x is where a moment falls along a track trackW wide, clamped to it.
func (w window) x(ms int64, trackW float32) float32 {
	ms = min(max(ms, w.start), w.end)
	return float32(ms-w.start) / float32(w.end-w.start) * trackW
}

// tick is a half hour of the ruler.
type tick struct {
	ms   int64
	hour bool
}

// ticks are the half hours the window holds, in local time.
func (w window) ticks() []tick {
	t := time.UnixMilli(w.start).Truncate(time.Minute)
	if m := t.Minute() % 30; m != 0 {
		t = t.Add(time.Duration(30-m) * time.Minute)
	}
	var out []tick
	for ; t.UnixMilli() <= w.end; t = t.Add(30 * time.Minute) {
		out = append(out, tick{ms: t.UnixMilli(), hour: t.Minute() == 0})
	}
	return out
}

// block is a programme placed on a track: x0..x1 clipped to it, with whether
// the programme goes on past either edge.
type block struct {
	programme
	x0, x1       float32
	clipL, clipR bool
	current      bool
	past         bool
}

// blocks are the programmes that meet the window, placed on a track trackW
// wide. The ones that start before the window are clipped at its left edge,
// the ones that end after it at its right.
func (w window) blocks(progs []programme, now int64, trackW float32) []block {
	var out []block
	for _, p := range progs {
		if p.endMs <= w.start || p.startMs >= w.end {
			continue
		}
		b := block{
			programme: p,
			x0:        w.x(p.startMs, trackW), x1: w.x(p.endMs, trackW),
			clipL: p.startMs < w.start, clipR: p.endMs > w.end,
			current: p.onAir(now), past: p.endMs <= now,
		}
		if b.x1-b.x0 < 1 {
			continue
		}
		out = append(out, b)
	}
	return out
}

// Text inside a block keeps clear of its edges, and of the now line.
const (
	blockPad   = 10
	nowClear   = 10
	minLabel   = 150 // a label right of the now line wants at least this much room
	minElapsed = 64  // the room the clipped start's marker needs
)

// label is where a block's text starts and how wide it may run, in track
// DIPs, and whether the block's start marker fits to the left of it.
type label struct {
	x0, w float32
	// marker: the block is clipped on the left and has the room for its
	// start time before the text.
	marker bool
}

// labelOf places the text of block b. A block that holds the now line puts it
// right of the line (the programme still to come), or, when that is the
// narrower side and short of room, left of it; the line never crosses the
// words.
func (b block) labelOf(nowX float32) label {
	if !b.current {
		return label{x0: b.x0 + blockPad, w: max(0, b.x1-b.x0-2*blockPad)}
	}
	ahead := b.x1 - nowX - nowClear - blockPad
	behind := nowX - b.x0 - nowClear - blockPad
	if ahead >= minLabel || ahead >= behind {
		return label{x0: nowX + nowClear, w: max(0, ahead), marker: b.clipL && behind+blockPad >= minElapsed}
	}
	return label{x0: b.x0 + blockPad, w: max(0, behind)}
}
