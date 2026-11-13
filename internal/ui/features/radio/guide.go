package radio

import (
	"strconv"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// guide is the board's frame of reference for one frame: the window of time
// its track shows, where now falls on it, and where the ruler's ticks are. It
// is taken again only when the board reads the clock (every 30 seconds) or
// its width changes, never per frame.
type guide struct {
	win    window
	trackW float32
	now    int64
	nowX   float32
	ticks  []tick
}

func newGuide(now int64, trackW float32) guide {
	w := windowAt(now, trackW)
	return guide{win: w, trackW: trackW, now: now, nowX: w.x(now, trackW), ticks: w.ticks()}
}

// look is how a cell is drawn.
type look int

const (
	// lookNamed is a show with a name of its own that has yet to start: a
	// fill one step off the ground.
	lookNamed look = iota
	// lookCurrent is the programme on air, a step heavier.
	lookCurrent
	// lookQuiet is a station's own hours, a programme that has ended, and
	// the lanes that say a station has no schedule or can't play: an outline
	// only.
	lookQuiet
)

// cell is one box of a station's lane, placed on its track.
type cell struct {
	key    string
	x0, x1 float32
	look   look
	// name is what a link to the station's page is called (a programme and
	// its slot); title and sub are what the box shows.
	name, title, sub string
	// left is the time left in the programme on air ("45 min left").
	left string
	// text is where the words go; from is the start of a programme that
	// began before the window, shown at the left edge.
	text label
	from string
	// more: the programme goes on past the window's right edge.
	more bool
	// off: the lane says why Sona can't play the station. info: the lane only
	// says something (that, or the tagline), so it is no link.
	off, info bool
	// progress is how far through the programme on air the now line is.
	progress *cellProgress
}

// cellProgress is a programme on air's progress: value the share of the
// whole programme (what assistive technology reads), drawn the share of the
// visible box the now line has crossed (what the bar paints).
type cellProgress struct {
	value, drawn float64
	text         string
}

// cellsFor is a station's lane: its programmes in the window as cells, or
// one quiet cell when it can't play (and why) or has no schedule to show
// (its tagline).
func cellsFor(s services.Station, schedule []services.StationEvent, g guide) []cell {
	room := max(0, g.trackW-2*blockPad)
	if !s.Playable {
		msg := kit.StationUnavailableText(s.Unavailable)
		if msg == "" {
			msg = i18n.T("Sona can't play this station")
		}
		return []cell{{key: "off", x1: g.trackW, look: lookQuiet, title: msg, off: true, info: true, text: label{x0: blockPad, w: room}}}
	}
	blocks := g.win.blocks(programmes(schedule), g.now, g.trackW)
	if len(blocks) == 0 {
		title := s.Tagline
		if title == "" {
			title = i18n.T("No schedule")
		}
		return []cell{{key: "tagline", x1: g.trackW, look: lookQuiet, info: true, title: title, text: label{x0: blockPad, w: room}}}
	}
	// Nothing to count down to when the station's own hours run to the end of
	// its schedule.
	_, counts := onAirSpan(s.Name, schedule, g.now)
	cells := make([]cell, 0, len(blocks))
	for _, b := range blocks {
		cl := cell{
			key: strconv.FormatInt(b.startMs, 10), x0: b.x0, x1: b.x1, more: b.clipR,
			title: programmeName(s.Name, s.Tagline, b.event), sub: slotLabel(b.span, g.now),
			text: b.labelOf(g.nowX),
		}
		if cl.title == "" {
			cl.title = s.Tagline
		}
		cl.name = i18n.T("{programme}, {slot}", "programme", b.event.Title, "slot", cl.sub)
		switch {
		case b.current:
			cl.look = lookCurrent
			if counts {
				left := i18n.FormatRunningTime(time.Duration(max(60_000, b.endMs-g.now)) * time.Millisecond)
				cl.left = i18n.T("{left} left", "left", left)
				if cl.text.w < 220 {
					cl.sub = "" // no room for the slot beside the time left
				}
				cl.progress = &cellProgress{
					value: progressOf(b.span, g.now), drawn: float64((g.nowX - b.x0) / (b.x1 - b.x0)), text: cl.left,
				}
			} else {
				cl.sub = ""
			}
			if cl.text.marker {
				cl.from = startLabel(b.event, g.now)
			}
		case !b.past && !same(b.event.Title, s.Name):
			cl.look = lookNamed
		default:
			cl.look = lookQuiet
			if b.past {
				cl.sub = ""
			}
		}
		cells = append(cells, cl)
	}
	return cells
}
