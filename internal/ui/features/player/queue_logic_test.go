package player

import (
	"reflect"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// pattern is the entries of a queue from a pattern: "u" a user's song, "a" a
// station or AutoPlay song (queue-lines.test.ts).
func queuePattern(s string) []bool {
	out := make([]bool, len(s))
	for i, c := range s {
		out[i] = c == 'a'
	}
	return out
}

func lineNames(lines []line) []any {
	var out []any
	for _, l := range lines {
		switch l.kind {
		case lineRow:
			out = append(out, l.qi)
		case lineHead:
			out = append(out, "head")
		default:
			out = append(out, "note")
		}
	}
	return out
}

func TestQueueLines(t *testing.T) {
	autoPlay := &services.QueueStation{AutoPlay: true}
	radio := &services.QueueStation{}
	for _, tc := range []struct {
		name                         string
		q                            string
		start                        int
		station                      *services.QueueStation
		note                         bool
		lines                        []any
		mine, auto, userEnd, headIdx int
	}{
		{"AutoPlay goes under its heading after the user's songs", "uuuaa", 0, autoPlay, false, []any{0, 1, 2, "head", 3, 4}, 2, 2, 3, 3},
		{"lists from the current song: played songs are left out", "uuuaa", 2, autoPlay, false, []any{2, "head", 3, 4}, 0, 2, 3, 1},
		{"songs queued while AutoPlay plays go before the rest of it", "uauaa", 1, autoPlay, false, []any{1, 2, "head", 3, 4}, 1, 2, 3, 2},
		{"a station of songs only: no heading", "aaa", 0, radio, false, []any{0, 1, 2}, 0, 2, 1, -1},
		{"a station the user started is headed after songs they queued", "auaa", 0, radio, false, []any{0, 1, "head", 2, 3}, 1, 2, 2, 2},
		{"the note ends the list while AutoPlay is on its way", "uu", 0, nil, true, []any{0, 1, "note"}, 1, 0, 2, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := queueLines(queuePattern(tc.q), tc.start, tc.station, tc.note)
			if !reflect.DeepEqual(lineNames(got.lines), tc.lines) {
				t.Errorf("lines %v, want %v", lineNames(got.lines), tc.lines)
			}
			if got.mine != tc.mine || got.auto != tc.auto || got.userEnd != tc.userEnd || got.head != tc.headIdx {
				t.Errorf("mine %d auto %d userEnd %d head %d, want %d %d %d %d", got.mine, got.auto, got.userEnd, got.head, tc.mine, tc.auto, tc.userEnd, tc.headIdx)
			}
		})
	}
}

func TestPartsOfNotesAutoPlayOnItsWay(t *testing.T) {
	items := []services.QueueItem{{UID: 1, ID: "i.1"}, {UID: 2, ID: "i.2"}}
	s := services.PlaybackState{Index: 0, AutoPlay: true, Repeat: "off"}
	if p, _ := partsOf(s, services.QueueState{Items: items}); lineNames(p.lines)[len(p.lines)-1] != "note" {
		t.Errorf("no note: %v", lineNames(p.lines))
	}
	for name, mutate := range map[string]func(*services.PlaybackState, *services.QueueState){
		"repeat":    func(s *services.PlaybackState, _ *services.QueueState) { s.Repeat = "all" },
		"off":       func(s *services.PlaybackState, _ *services.QueueState) { s.AutoPlay = false },
		"a station": func(_ *services.PlaybackState, q *services.QueueState) { q.Station = &services.QueueStation{} },
		"radio only": func(_ *services.PlaybackState, q *services.QueueState) {
			q.Items = []services.QueueItem{{UID: 1, ID: "ra.1"}}
		},
	} {
		s2, q2 := s, services.QueueState{Items: items}
		mutate(&s2, &q2)
		if p, _ := partsOf(s2, q2); lineNames(p.lines)[len(p.lines)-1] == "note" {
			t.Errorf("%s: a note without AutoPlay on its way", name)
		}
	}
}

func queueUIDs(u ...uint64) []services.QueueItem {
	out := make([]services.QueueItem, len(u))
	for i, id := range u {
		out[i].UID = id
	}
	return out
}

func TestFocusedRow(t *testing.T) {
	five := queueUIDs(10, 11, 12, 13, 14)
	for _, tc := range []struct {
		name  string
		items []services.QueueItem
		start int
		focus *focusTarget
		want  int
	}{
		{"starts on the current song", five, 1, nil, 1},
		{"follows the entry when it moves", queueUIDs(10, 12, 11, 13, 14), 0, &focusTarget{11, 1}, 2},
		{"the next row when the focused one is removed", queueUIDs(10, 11, 13, 14), 0, &focusTarget{12, 2}, 2},
		{"the last one when it was last", queueUIDs(10, 11, 12, 13), 0, &focusTarget{14, 4}, 3},
		{"the current song when the focused one was played", five, 3, &focusTarget{11, 1}, 3},
		{"no row when nothing is listed", nil, 0, nil, -1},
		{"no row past the end", five, 5, &focusTarget{14, 4}, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := focusedRow(tc.items, tc.start, tc.focus); got != tc.want {
				t.Errorf("focusedRow = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRowForKey(t *testing.T) {
	for _, tc := range []struct {
		key        string
		qi         int
		want       int
		wantOK     bool
		start, end int
	}{
		{"ArrowDown", 2, 3, true, 1, 5},
		{"ArrowUp", 2, 1, true, 1, 5},
		{"Home", 3, 1, true, 1, 5},
		{"End", 1, 4, true, 1, 5},
		{"ArrowUp", 1, 1, true, 1, 5},   // stops at the top: no played songs
		{"ArrowDown", 4, 4, true, 1, 5}, // and at the end
		{"Enter", 2, 0, false, 1, 5},
	} {
		got, ok := rowForKey(tc.key, tc.qi, tc.start, tc.end)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("rowForKey(%s, %d) = %d, %v; want %d, %v", tc.key, tc.qi, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestMoveForKey(t *testing.T) {
	for _, tc := range []struct {
		key    string
		qi     int
		want   int
		wantOK bool
	}{
		{"ArrowUp", 2, 1, true},
		{"ArrowDown", 2, 3, true},
		{"ArrowDown", 4, 5, false}, // not into AutoPlay's part
		{"ArrowUp", 0, -1, false},
		{"Home", 2, 0, false},
	} {
		got, ok := moveForKey(tc.key, tc.qi, 5)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Errorf("moveForKey(%s, %d) = %d, %v; want %d, %v", tc.key, tc.qi, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestMenuMoves(t *testing.T) {
	// Playing queue[1]; the user's songs are queue[2..4], AutoPlay's start at 5.
	for _, tc := range []struct {
		name                string
		qi, index, userEnd  int
		auto, next, wantEnd bool
	}{
		{"both for a song in the middle", 3, 1, 5, false, true, true},
		{"not Play next for the song already next", 2, 1, 5, false, false, true},
		{"not Move to end for the last of the user's", 4, 1, 5, false, true, false},
		{"neither with one song queued", 2, 1, 3, false, false, false},
		{"neither for the playing song", 1, 1, 5, false, false, false},
		{"neither for an AutoPlay song", 6, 1, 5, true, false, false},
		{"from the first song with nothing playing (first)", 0, -1, 3, false, false, true},
		{"from the first song with nothing playing (second)", 1, -1, 3, false, true, true},
	} {
		next, end := menuMoves(tc.qi, tc.index, tc.userEnd, tc.auto)
		if next != tc.next || end != tc.wantEnd {
			t.Errorf("%s: menuMoves = %v, %v; want %v, %v", tc.name, next, end, tc.next, tc.wantEnd)
		}
	}
}

// The panel's geometry: line tops (the heading is taller than a row), the
// gap under the pointer, and where the drop line goes.
func TestQueueGeometry(t *testing.T) {
	parts := queueLines(queuePattern("uuuaa"), 0, &services.QueueStation{AutoPlay: true}, false)
	g := queueGeom{lines: parts.lines, head: parts.head, start: 0, userEnd: parts.userEnd, n: 5, first: 1}
	if got := g.total(); got != 5*queueRowH+queueHeadH {
		t.Errorf("total = %v", got)
	}
	// 0 1 2 head 3 4: the heading's line starts at 156, row 3 after it at 216.
	if got := g.top(3); got != 3*queueRowH {
		t.Errorf("top(head) = %v", got)
	}
	if got := g.top(4); got != 3*queueRowH+queueHeadH {
		t.Errorf("top(row 3) = %v", got)
	}
	if got := g.lineOf(3); got != 4 {
		t.Errorf("lineOf(3) = %d", got)
	}
	for _, tc := range []struct {
		off  float32
		want int
	}{
		{0, 1},                     // nothing goes before the current song
		{queueRowH + 10, 1},        // top half of row 1: before it
		{queueRowH + 40, 2},        // bottom half: after it
		{2*queueRowH + 40, 3},      // after the last of the user's songs
		{3*queueRowH + 30, 3},      // on the heading: the end of the user's part
		{3*queueRowH + 80, 3},      // in the station's part: clamped
		{10 * queueRowH, 3},        // below everything
		{2*queueRowH + 10, 2},      // top half of row 2
		{queueRowH + queueRowH, 2}, // the seam between rows 1 and 2
	} {
		if got := g.slotAt(tc.off); got != tc.want {
			t.Errorf("slotAt(%v) = %d, want %d", tc.off, got, tc.want)
		}
	}
	if got := g.slotTop(2); got != 2*queueRowH {
		t.Errorf("slotTop(2) = %v", got)
	}
	if got := g.slotTop(3); got != 3*queueRowH {
		t.Errorf("slotTop(3) = the heading's top: %v", got)
	}
}
