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
)

// line is one line of the queue panel.
type line struct {
	kind lineKind
	qi   int
}

// queueParts is the queue as the panel shows it: the current song and the
// user's songs, then the station's or AutoPlay's part under its own
// heading, then (while AutoPlay is still on its way) a note.
type queueParts struct {
	lines []line
	// mine and auto count the user's and the station's songs after the
	// current one.
	mine, auto int
	// userEnd is where the user's part ends: the first upcoming station
	// slot, else the end.
	userEnd int
	// head is the heading's line, -1 without one; the rows after it are the
	// station's part.
	head int
}

// queueLines builds the lines for entries from start (the current song). The
// station's part gets a heading when it follows songs the user queued, and
// always for AutoPlay (it is not what the user chose). note adds the AutoPlay
// note.
func queueLines(auto []bool, start int, station *services.QueueStation, note bool) queueParts {
	q := queueParts{userEnd: len(auto), head: -1}
	for qi := start; qi < len(auto); qi++ {
		upcoming := qi > start
		if upcoming && auto[qi] && q.auto == 0 {
			q.userEnd = qi
			if station != nil && (station.AutoPlay || q.mine > 0) {
				q.head = len(q.lines)
				q.lines = append(q.lines, line{kind: lineHead})
			}
		}
		if upcoming {
			if auto[qi] {
				q.auto++
			} else {
				q.mine++
			}
		}
		q.lines = append(q.lines, line{kind: lineRow, qi: qi})
	}
	if note {
		q.lines = append(q.lines, line{kind: lineNote})
	}
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
