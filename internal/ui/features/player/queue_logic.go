package player

import (
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The queue list's pure logic (v0.2.0 queue-lines.ts and queue-focus.ts).

// lineKind is what one line of the queue panel is.
type lineKind int

const (
	lineRow  lineKind = iota // a song: queue index qi
	lineHead                 // the heading of the station's or AutoPlay's part
	lineNote                 // "Then AutoPlay continues with similar music"
	lineNow                  // the heading of the playing song's group
	lineNext                 // the heading of what the user queued to play next
)

// The heights of the queue's lines. The playing song is a larger row under
// its own heading; the headings of Next and of the station's or AutoPlay's
// part carry a line of detail and have room above them.
const (
	queueRowH     = 52
	queueNowH     = 72
	queueNowHeadH = 36
	queueHeadH    = 60
	queueNoteH    = 44
)

// line is one line of the queue panel, y being where it starts in the body.
type line struct {
	kind lineKind
	qi   int
	y    float32
	// cur: the row of the playing song.
	cur bool
}

func (l *line) height() float32 {
	switch l.kind {
	case lineRow:
		if l.cur {
			return queueNowH
		}
		return queueRowH
	case lineNow:
		return queueNowHeadH
	case lineHead, lineNext:
		return queueHeadH
	}
	return queueNoteH
}

// queueParts is the queue as the panel shows it: the playing song under its
// heading, Next with the songs the user queued, then the station's or
// AutoPlay's part under its own heading, then (while AutoPlay is still on its
// way) a note.
type queueParts struct {
	lines []line
	// mine and auto count the user's and the station's songs after the
	// current one.
	mine, auto int
	// userEnd is where the user's part ends: the first upcoming station
	// slot, else the end.
	userEnd int
	// head is the station's heading line, -1 without one; the rows after it
	// are the station's part.
	head int
	// rowAt is the line of each queue row, from the current song's.
	rowAt []int
	// total is the lines' height.
	total float32
}

// queueLines builds the lines for entries from start (the current song). The
// station's part gets a heading when it follows songs the user queued, and
// always for AutoPlay (it is not what the user chose); where it follows the
// playing song directly and has none, Next heads it. note adds the AutoPlay
// note.
func queueLines(auto []bool, start int, station *services.QueueStation, note bool) queueParts {
	q := queueParts{userEnd: len(auto), head: -1}
	have := start < len(auto)
	if have {
		q.lines = append(q.lines, line{kind: lineNow})
	}
	for qi := start; qi < len(auto); qi++ {
		upcoming := qi > start
		stationHead := false
		if upcoming && auto[qi] && q.auto == 0 {
			q.userEnd = qi
			stationHead = station != nil && (station.AutoPlay || q.mine > 0)
		}
		switch {
		case stationHead:
			q.head = len(q.lines)
			q.lines = append(q.lines, line{kind: lineHead})
		case qi == start+1:
			q.lines = append(q.lines, line{kind: lineNext})
		}
		if upcoming {
			if auto[qi] {
				q.auto++
			} else {
				q.mine++
			}
		}
		q.lines = append(q.lines, line{kind: lineRow, qi: qi, cur: qi == start})
	}
	if have && start+1 >= len(auto) {
		q.lines = append(q.lines, line{kind: lineNext}) // "Nothing after this song"
	}
	if note {
		q.lines = append(q.lines, line{kind: lineNote})
	}
	var y float32
	for i := range q.lines {
		l := &q.lines[i]
		l.y = y
		y += l.height()
		if l.kind == lineRow {
			q.rowAt = append(q.rowAt, i)
		}
	}
	q.total = y
	return q
}

// partsOf derives the panel's parts from the mirror's snapshots.
func partsOf(s services.PlaybackState, q services.QueueState) (parts queueParts, start int) {
	start = max(0, s.Index)
	auto := make([]bool, len(q.Items))
	continues := false
	for i, it := range q.Items {
		auto[i] = it.Auto
		// AutoPlay needs a song to continue from; radio streams ("ra.…") are not songs.
		if !strings.HasPrefix(it.ID, "ra.") {
			continues = true
		}
	}
	continues = continues && s.AutoPlay && q.Station == nil && s.Repeat == "off"
	return queueLines(auto, start, q.Station, continues), start
}

// focusTarget is the queue row with the list's Tab stop, remembered by its
// entry's uid, with its last index for when the entry is gone.
type focusTarget struct {
	uid uint64
	qi  int
}

// focusedRow is the queue index of the focused row: the remembered entry
// where it still is, else the row now at its old place (the next one after
// a removal), else the current song. -1 when the list has no rows.
func focusedRow(items []services.QueueItem, start int, focus *focusTarget) int {
	if start >= len(items) {
		return -1
	}
	if focus != nil {
		for i, it := range items {
			if it.UID == focus.uid {
				if i >= start {
					return i
				}
				break
			}
		}
	}
	at := start
	if focus != nil {
		at = focus.qi
	}
	return max(start, min(len(items)-1, at))
}

// rowForKey is where Up, Down, Home and End move focus from row qi; ok is
// false for other keys.
func rowForKey(key string, qi, start, end int) (to int, ok bool) {
	switch key {
	case "ArrowDown":
		return min(end-1, qi+1), true
	case "ArrowUp":
		return max(start, qi-1), true
	case "Home":
		return start, true
	case "End":
		return end - 1, true
	}
	return 0, false
}

// moveForKey is where Alt+Up or Alt+Down moves the user's song at qi: one
// step, staying before the station's part (which starts at userEnd); ok is
// false when it can't move that way.
func moveForKey(key string, qi, userEnd int) (to int, ok bool) {
	switch key {
	case "ArrowUp":
		to = qi - 1
	case "ArrowDown":
		to = qi + 1
	default:
		return 0, false
	}
	return to, to >= 0 && to < userEnd
}

// menuMoves says which moves the row menu offers for the song at qi: to the
// front of what plays next, and to the end of the user's songs. Neither for
// the playing song or a station or AutoPlay song; each is left out where it
// would not change the order. index is the current song (-1 with none).
func menuMoves(qi, index, userEnd int, auto bool) (next, end bool) {
	if auto || qi <= index {
		return false, false
	}
	return qi > index+1, qi < userEnd-1
}

// partsKey is what partsOf reads: the cached parts are valid while it holds.
type partsKey struct {
	version    uint64
	n, index   int
	autoPlay   bool
	repeat     string
	station    services.QueueStation
	hasStation bool
}

// partsCache is partsOf's last answer: the summary, the tools and the list
// each ask every frame, and the queue changes far less often.
type partsCache struct {
	key   partsKey
	parts queueParts
	start int
	valid bool
}

// queueParts is partsOf, computed once per change of the queue and of what
// it reads. The result is shared: callers don't change it.
func (p *player) queueParts(s services.PlaybackState, q services.QueueState) (queueParts, int) {
	k := partsKey{version: q.Version, n: len(q.Items), index: s.Index, autoPlay: s.AutoPlay, repeat: s.Repeat}
	if q.Station != nil {
		k.station, k.hasStation = *q.Station, true
	}
	c := &p.queue.parts
	if !c.valid || c.key != k {
		c.parts, c.start = partsOf(s, q)
		c.key, c.valid = k, true
	}
	return c.parts, c.start
}
