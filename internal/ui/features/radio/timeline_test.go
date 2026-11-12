package radio

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The guide's arithmetic: windows, boxes and where their words go. t0 and hourMs
// are schedule_test.go's.

const minMs = int64(time.Minute / time.Millisecond)

func TestProgrammesJoinBackToBackSlotsOfOneTitleOnly(t *testing.T) {
	got := programmes([]services.StationEvent{
		ev("own", t0, t0+hourMs), ev("own", t0+hourMs, t0+2*hourMs),
		ev("show", t0+2*hourMs, t0+3*hourMs),
		ev("own", t0+4*hourMs, t0+5*hourMs), // a gap: not joined to the one before
		ev("own", t0+5*hourMs, t0+6*hourMs),
	})
	want := []span{{t0, t0 + 2*hourMs}, {t0 + 2*hourMs, t0 + 3*hourMs}, {t0 + 4*hourMs, t0 + 6*hourMs}}
	if len(got) != len(want) {
		t.Fatalf("%d programmes, want %d: %v", len(got), len(want), got)
	}
	for i, p := range got {
		if p.span != want[i] {
			t.Errorf("programme %d is %v, want %v", i, p.span, want[i])
		}
	}
	if !got[0].onAir(t0+90*minMs) || got[0].onAir(t0+2*hourMs) {
		t.Error("a joined programme is on air from its first slot's start to its last slot's end")
	}
}

func TestWindowStartsHalfAnHourBeforeNowAndGrowsWithTheTrack(t *testing.T) {
	now := t0 + 15*minMs + 40_000 // 10:15:40
	w := windowAt(now, 830)
	if want := t0 - 15*minMs; w.start != want {
		t.Errorf("the window starts at %v, want 09:45:00", time.UnixMilli(w.start))
	}
	if got := (w.end - w.start) / minMs; got != 240 {
		t.Errorf("a track of 830 DIPs holds %d minutes, want 240", got)
	}
	if got := (windowAt(now, 412).end - windowAt(now, 412).start) / minMs; got != 120 {
		t.Errorf("the narrowest track holds %d minutes, want 120", got)
	}
	if got := (windowAt(now, 100).end - windowAt(now, 100).start) / minMs; got != 90 {
		t.Errorf("a track of 100 DIPs holds %d minutes, want the floor of 90", got)
	}
	if got := (windowAt(now, 4000).end - windowAt(now, 4000).start) / minMs; got != 300 {
		t.Errorf("a track of 4000 DIPs holds %d minutes, want the ceiling of 300", got)
	}
	// The window moves by the minute, so the seconds only move the now line.
	if windowAt(now-40_000, 830) != w {
		t.Errorf("the window moved with the seconds: %v and %v", windowAt(now-40_000, 830), w)
	}
}

func TestTicksAreTheHalfHoursOfTheWindowAndHoursAreMarked(t *testing.T) {
	w := windowAt(t0+15*minMs, 830) // 09:45 to 13:45
	ticks := w.ticks()
	if len(ticks) != 8 {
		t.Fatalf("%d ticks, want 8: %v", len(ticks), ticks)
	}
	if ticks[0].ms != t0 || !ticks[0].hour {
		t.Errorf("the first tick is %v (hour %v), want 10:00, the first half hour inside the window", time.UnixMilli(ticks[0].ms), ticks[0].hour)
	}
	for i, tk := range ticks {
		if got := time.UnixMilli(tk.ms).Minute() == 0; got != tk.hour {
			t.Errorf("tick %d at %v: hour %v", i, time.UnixMilli(tk.ms), tk.hour)
		}
	}
}

func TestBlocksAreClippedAtTheWindowsEdgesAndLeaveOutWhatIsOutside(t *testing.T) {
	now := t0 + 15*minMs
	w := windowAt(now, 830) // 09:45 to 13:45, 3.46 DIPs a minute
	progs := programmes([]services.StationEvent{
		ev("gone", t0-3*hourMs, t0-2*hourMs),
		ev("before", t0-hourMs, t0+hourMs), // started before the window, on air
		ev("next", t0+hourMs, t0+2*hourMs),
		ev("long", t0+2*hourMs, t0+9*hourMs), // runs past the window's end
		ev("far", t0+10*hourMs, t0+11*hourMs),
	})
	blocks := w.blocks(progs, now, 830)
	if len(blocks) != 3 {
		t.Fatalf("%d blocks, want 3: %v", len(blocks), blocks)
	}
	b := blocks[0]
	if b.x0 != 0 || !b.clipL || b.clipR || !b.current || b.past {
		t.Errorf("the programme on air is %+v, want clipped at the left edge", b)
	}
	if got, want := b.x1, w.x(t0+hourMs, 830); got != want {
		t.Errorf("it ends at %v, want %v", got, want)
	}
	if blocks[1].current || blocks[1].past || blocks[1].clipL || blocks[1].clipR {
		t.Errorf("the next programme is %+v", blocks[1])
	}
	if last := blocks[2]; !last.clipR || last.x1 != 830 {
		t.Errorf("the long programme is %+v, want clipped at the right edge", last)
	}
	// One that ended inside the window is drawn, as past.
	ended := w.blocks(programmes([]services.StationEvent{ev("ended", t0-hourMs, t0+5*minMs)}), now+30*minMs, 830)
	if len(ended) != 1 || !ended[0].past {
		t.Errorf("a programme that ended 25 minutes ago: %+v", ended)
	}
}

func TestLabelsKeepClearOfTheNowLine(t *testing.T) {
	// On air, started well before the window: the words go right of the line.
	b := block{x0: 0, x1: 300, current: true, clipL: true}
	l := b.labelOf(110)
	if l.x0 != 120 || l.w != 300-110-nowClear-blockPad || !l.marker {
		t.Errorf("label right of the line = %+v", l)
	}
	// Nearly over: the room is behind the line, and the words go there.
	b = block{x0: 0, x1: 150, current: true}
	l = b.labelOf(120)
	if l.x0 != blockPad || l.x0+l.w > 120-nowClear {
		t.Errorf("label left of the line = %+v, it would cross the line at 120", l)
	}
	// A box the line does not cross has its words from its own edge.
	b = block{x0: 200, x1: 400}
	if l := b.labelOf(110); l.x0 != 200+blockPad || l.w != 200-2*blockPad || l.marker {
		t.Errorf("label of a later box = %+v", l)
	}
	// A box just started: no room before the line for a marker.
	b = block{x0: 100, x1: 400, current: true, clipL: true}
	if l := b.labelOf(120); l.marker {
		t.Errorf("a marker in %v DIPs: %+v", 120-100, l)
	}
}

func TestCellsOfALane(t *testing.T) {
	now := t0 + 15*minMs
	g := newGuide(now, 830)
	one := liveStation("ra.1", "Apple Music 1")
	// A show on air from before the window, the station's own hour, then a show.
	cells := cellsFor(one, []services.StationEvent{
		stationEvent("The Zane Lowe Show", time.UnixMilli(t0-hourMs), time.UnixMilli(t0+hourMs), "New music."),
		stationEvent("Apple Music 1", time.UnixMilli(t0+hourMs), time.UnixMilli(t0+2*hourMs)),
		stationEvent("The Matt Wilkinson Show", time.UnixMilli(t0+2*hourMs), time.UnixMilli(t0+3*hourMs)),
	}, g)
	if len(cells) != 3 {
		t.Fatalf("%d cells: %+v", len(cells), cells)
	}
	cur, own, show := cells[0], cells[1], cells[2]
	if cur.look != lookCurrent || cur.title != "The Zane Lowe Show" || cur.progress == nil || cur.left == "" {
		t.Errorf("the programme on air: %+v", cur)
	}
	if cur.from == "" {
		t.Errorf("a programme that began before the window shows its start at the edge: %+v", cur)
	}
	if cur.progress != nil && (cur.progress.value != 0.625 || cur.progress.drawn <= 0 || cur.progress.drawn >= 1) {
		t.Errorf("progress = %+v, want 62.5%% of the programme and part of the visible box", cur.progress)
	}
	if own.look != lookQuiet || own.title != one.Tagline {
		t.Errorf("the station's own hour is a quiet box with the tagline: %+v", own)
	}
	if show.look != lookNamed || show.title != "The Matt Wilkinson Show" || show.sub == "" {
		t.Errorf("the show to come: %+v", show)
	}
	if show.x0 < own.x1 {
		t.Errorf("boxes overlap: %v < %v", show.x0, own.x1)
	}
}

func TestCellsOfALaneThatSaysSomethingInstead(t *testing.T) {
	g := newGuide(t0, 830)
	off := liveStation("ra.4", "Off", func(s *services.Station) { s.Playable, s.Unavailable = false, services.StationNoWidevine })
	cells := cellsFor(off, []services.StationEvent{ev("show", t0, t0+hourMs)}, g)
	if len(cells) != 1 || !cells[0].off || !cells[0].info || cells[0].x1 != g.trackW {
		t.Fatalf("a station that can't play is one lane saying why: %+v", cells)
	}
	if cells[0].title == "" {
		t.Error("the lane does not say why")
	}
	bare := cellsFor(liveStation("ra.5", "Bare"), nil, g)
	if len(bare) != 1 || bare[0].title != "Bare tagline" || !bare[0].info {
		t.Errorf("a station without a schedule shows its tagline: %+v", bare)
	}
	none := liveStation("ra.6", "Quiet", func(s *services.Station) { s.Tagline = "" })
	if got := cellsFor(none, nil, g); len(got) != 0 {
		t.Errorf("a lane with nothing to say has cells: %+v", got)
	}
	// Own hours to the end of the schedule have nothing to count down to.
	hits := liveStation("ra.2", "Hits")
	run := cellsFor(hits, []services.StationEvent{ev("Hits", t0-hourMs, t0+hourMs)}, newGuide(t0+5*minMs, 830))
	if len(run) != 1 || run[0].progress != nil || run[0].left != "" {
		t.Errorf("own hours to the schedule's end count down: %+v", run)
	}
}
