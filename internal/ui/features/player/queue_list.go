package player

import (
	"sort"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// queueOverscan is how many lines are built beyond those showing.
	queueOverscan = 8
	// queueRetries is how many frames a requested focus waits for its row
	// to be built.
	queueRetries = 10
)

// queueGeom is where the lines of the queue are, as the last build laid
// them out: the drop line and the pointer's gap are placed by it.
type queueGeom struct {
	lines []line
	// rowAt is the line of each queue row, from the current song's.
	rowAt []int
	// head is the station's heading line (-1 without), start the current
	// song's queue index, userEnd where the user's part ends, n the queue's
	// length.
	head, start, userEnd, n int
	// first is the first slot a drop may land in: nothing goes before the
	// current song (with none, anywhere).
	first int
}

func (g *queueGeom) height(i int) float32 { return g.lines[i].height() }

// top is where line i starts in the body.
func (g *queueGeom) top(i int) float32 { return g.lines[i].y }

// total is the body's height.
func (g *queueGeom) total() float32 {
	if len(g.lines) == 0 {
		return 0
	}
	last := len(g.lines) - 1
	return g.top(last) + g.height(last)
}

// lineOf is the line of the queue row qi.
func (g *queueGeom) lineOf(qi int) int { return g.rowAt[qi-g.start] }

// groupOf is the group of queue row qi: 0 the playing song, 1 Next, 2 the
// station's or AutoPlay's part. The groups are lists of their own, so a row
// that changes group is built again.
func (g *queueGeom) groupOf(qi int) int {
	switch {
	case qi == g.start:
		return 0
	case g.head >= 0 && qi >= g.userEnd:
		return 2
	}
	return 1
}

// lineAt is the line at body offset off (the last one for an offset below
// them all).
func (g *queueGeom) lineAt(off float32) int {
	i := sort.Search(len(g.lines), func(i int) bool { return g.lines[i].y > off })
	return max(i-1, 0)
}

// slotAt is the gap a drop at off (from the body's top) lands in: before a
// row in its top half, after it in its bottom half, clamped between the
// current song and the end of the user's part.
func (g *queueGeom) slotAt(off float32) int {
	s := g.userEnd
	for i, l := range g.lines {
		top, h := g.top(i), g.height(i)
		if off >= top+h {
			continue
		}
		switch l.kind {
		case lineRow:
			s = l.qi + 1
			if off < top+h/2 {
				s = l.qi
			}
		case lineNow, lineNext:
			s = g.first
		}
		break
	}
	return max(g.first, min(g.userEnd, s))
}

// slotTop is where the gap before queue[slot] is in the body: the top of its
// row, or of the station's heading where the user's part ends.
func (g *queueGeom) slotTop(slot int) float32 {
	for i, l := range g.lines {
		switch l.kind {
		case lineNow, lineNext:
		case lineRow:
			if l.qi >= slot {
				return g.top(i)
			}
		default:
			return g.top(i)
		}
	}
	return g.total()
}

// queueList is one list's own state.
type queueList struct {
	pos ui.ScrollState
	// paneH is the scroller's height and body the lines' box (the last
	// frame's): the window to build and the pointer's gap come from them.
	paneH float32
	body  ui.Rect
	g     queueGeom
	// slot is the gap a drop would land in (before queue[slot]) while a drag
	// is over the list.
	slot   int
	slotOn bool
	// want is the queue row asked to take the focus, plus one; 0 none.
	want, tries int
	// focusedNow is the entry whose row has the focus in this frame's build,
	// focusedWas the last frame's.
	focusedNow, focusedWas uint64
	// focusedGroup: the group (queueGeom.groupOf) of the focused row last
	// frame.
	focusedGroup int
	menuWas      bool
	last         int64
}

// focusRow asks for the focus to go to queue row qi: scrolled into view
// when it is not built, and taken by the row when it builds.
func (p *player) focusRow(st *queueList, items []services.QueueItem, qi int) {
	if qi < 0 || qi >= len(items) {
		return
	}
	p.queue.focus = &focusTarget{uid: items[qi].UID, qi: qi}
	st.want, st.tries = qi+1, 0
	st.scrollTo(qi)
}

// scrollTo scrolls the list as little as shows row qi.
func (st *queueList) scrollTo(qi int) {
	g := &st.g
	if st.paneH <= 0 || qi < g.start || qi >= g.n || len(g.lines) == 0 {
		return
	}
	i := g.lineOf(qi)
	top, bottom := g.top(i), g.top(i)+g.height(i)
	if i > 0 && g.lines[i-1].kind != lineRow {
		top = g.top(i - 1) // the heading above it stays in view
	}
	switch {
	case top < st.pos.Y:
		st.pos.Y = max(0, top)
	case bottom > st.pos.Y+st.paneH:
		st.pos.Y = bottom - st.paneH
	}
}

// queueAccepts is what the queue takes: its own rows, and songs, music
// videos, albums and playlists from anywhere.
func queueAccepts(p *kit.DragPayload) bool {
	if p.Origin != nil && p.Origin.List == queueDropID {
		return true
	}
	for _, it := range p.Items {
		switch it.Kind {
		case kit.KindSong, kit.KindMusicVideo, kit.KindAlbum, kit.KindPlaylist:
			return true
		}
	}
	return false
}

// queueTarget is the queue's drop target: a drop on it moves a row to the
// gap under the pointer, or adds what was dragged there (after the user's
// songs when the queue is empty).
func (p *player) queueTarget(c *ui.Context, st *queueList) kit.DropTarget {
	return kit.DropTarget{
		Accepts: queueAccepts,
		Describe: func(pl *kit.DragPayload) string {
			if pl.Origin != nil && pl.Origin.List == queueDropID {
				return i18n.T("Move here")
			}
			return i18n.T("Add to the queue")
		},
		Hover: func(_ *kit.DragPayload, at *kit.DragPoint) {
			slot, on := 0, false
			if at != nil && st.g.n > 0 && st.body.H > 0 {
				slot, on = st.g.slotAt(at.Y-st.body.Y), true
			}
			if slot != st.slot || on != st.slotOn {
				st.slot, st.slotOn = slot, on
				c.Invalidate()
			}
		},
		Drop: func(pl *kit.DragPayload, at kit.DragPoint) {
			slot, on := 0, false
			if st.g.n > 0 && st.body.H > 0 {
				slot, on = st.g.slotAt(at.Y-st.body.Y), true
			}
			if pl.Origin != nil && pl.Origin.List == queueDropID {
				items := p.env.Play.Queue().Items
				from := pl.Origin.Index
				if from < 0 || from >= len(items) {
					return
				}
				to := from
				if on {
					to = kit.DropIndex(from, slot)
				}
				if to != from {
					p.askMove(items[from], from, false, func() error { return p.svc.Move(from, to) })
				}
				return
			}
			if on {
				p.env.Queue.QueueItems(pl.Items, kit.QueueAt(slot))
				return
			}
			p.env.Queue.QueueItems(pl.Items, kit.QueueLast)
		},
	}
}

// queueList is the queue's lists: the current song, then what plays next,
// the user's songs first and then, under their own heading, what the
// station or AutoPlay adds (AutoPlay songs set back: they keep their place,
// and playing one jumps there). The lists are one Tab stop: the arrows,
// Home and End move between songs, Alt+Up and Alt+Down or a drag reorder the
// user's songs, Delete removes, Enter or a double click jumps. Songs, albums
// and playlists dragged from a page drop into the gap under the pointer (or
// after the user's songs). Shown beside the page (queueAside) and as Now
// Playing's Up next (inView, which builds its own drop target and leaves the
// summary to the view).
//
// The lines are built by hand over Scroll and TrackScroll: only those
// showing, and eight beyond, are built; spacers hold the rest of the height.
func (p *player) queueList(c *ui.Context, inView bool) {
	st := &p.queue.lists[0]
	if inView {
		st = &p.queue.lists[1]
	}
	s := p.env.Play.State()
	q := p.env.Play.Queue()
	items := q.Items
	parts, start := p.queueParts(s, q)
	px := float32(8)
	if inView {
		px = 4
	}

	// A list that was not built for a while is not the one that had the focus.
	now := c.Now().UnixMilli()
	if now-st.last > 1000 {
		st.focusedWas = 0
	}
	st.last = now

	root := ui.Column(c.Key("queue-list")).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	over := p.queue.over
	if inView {
		over = kit.DropOn(root, queueDropID, p.queueTarget(c, st))
	}
	root.Children(func() {
		if len(items) == 0 {
			p.queueEmpty(c, inView, over)
			st.g = queueGeom{head: -1}
			return
		}
		p.queueBody(c, st, inView, over, px, s, q, parts, start)
	})
	p.settle(c, st, items, &st.g)
	p.rowMenu(c, st, items, s.Index)
}

// queueEmpty is the list with nothing in it: a dashed box that says how to
// start a queue, and "Drop to start a queue" while a drag is over it.
func (p *player) queueEmpty(c *ui.Context, inView, over bool) {
	tk := theme.Current()
	mx := float32(16)
	if inView {
		mx = 0
	}
	box := ui.Column(c.Key("empty")).Margin(8, mx, 0, mx).Padding(20, 16).Radius(tk.Radius["lg"]).Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed)
	title := i18n.T("The queue is empty")
	if over {
		box.Border(1, theme.C("ink").Alpha(0.4)).Background(theme.C("hover"))
		title = i18n.T("Drop to start a queue")
	}
	box.Children(func() {
		label(c, title, "ui").FontWeight(500).TextColor(theme.C("ink"))
		t := tk.Type["sub"]
		ui.Text(c, i18n.T("Play an album, playlist or station, choose Play next or Play last on any song, or drag songs here.")).
			FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0)
	})
}

// queueBody is the scroller with the lines and the drop line.
func (p *player) queueBody(c *ui.Context, st *queueList, inView, over bool, px float32, s services.PlaybackState, q services.QueueState, parts queueParts, start int) {
	items := q.Items
	station := q.Station
	index := s.Index
	g := queueGeom{lines: parts.lines, rowAt: parts.rowAt, head: parts.head, start: start, userEnd: parts.userEnd, n: len(items), first: min(index+1, len(items))}
	st.g = g
	playing := s.State == store.StatePlaying || s.State == store.StateLoading
	withheldOK := p.restricted()
	autoPlay := station != nil && station.AutoPlay

	headTitle, headDetail := "", ""
	if station != nil {
		if station.AutoPlay {
			headTitle = i18n.T("AutoPlay")
			// translators: Under the AutoPlay heading in the queue. Apple: ASE.Web.Music.UpNext.Autoplay.Description
			headDetail = i18n.T("Similar music will keep playing")
		} else {
			headTitle = i18n.T("Then {stationName}", "stationName", station.Name)
			headDetail = i18n.T("The station continues after your songs")
		}
	}

	// The window: the lines showing and queueOverscan beyond, and the rows
	// that must stay built (the one dragged, the one with the focus).
	h := st.paneH
	if h <= 0 {
		h = queueRowH * 20
	}
	wFirst := max(0, g.lineAt(st.pos.Y)-queueOverscan)
	wLast := min(len(g.lines)-1, g.lineAt(st.pos.Y+h)+queueOverscan)
	pin1, pin2 := -1, -1
	dragFrom, dragging := kit.DragPinned(st)
	if dragging && dragFrom >= start && dragFrom < len(items) {
		pin1 = g.lineOf(dragFrom)
	}
	focusQi := focusedRow(items, start, p.queue.focus)
	if st.focusedWas != 0 && focusQi >= 0 {
		pin2 = g.lineOf(focusQi)
	}
	built := func(i int) bool { return i >= wFirst && i <= wLast || i == pin1 || i == pin2 }

	// The Tab stop must be built: while the focused row is out of the
	// window the first built row holds it.
	tabQi := -1
	for i, l := range g.lines {
		if l.kind != lineRow || !built(i) {
			continue
		}
		if l.qi == focusQi {
			tabQi = l.qi
			break
		}
		if tabQi < 0 {
			tabQi = l.qi
		}
	}

	// The groups, in order: the playing song under its heading, Next (the
	// songs after it; the station's too while it has no part of its own), the
	// station's or AutoPlay's part under its heading, the note.
	rowsEnd := len(g.lines)
	if rowsEnd > 0 && g.lines[rowsEnd-1].kind == lineNote {
		rowsEnd--
	}
	nowRow := -1 // the playing song's line
	nextAt := 0  // the first line after it
	if len(g.lines) > 0 && g.lines[0].kind == lineNow {
		nowRow, nextAt = 1, 2
	}
	nextHead := nextAt < rowsEnd && g.lines[nextAt].kind == lineNext
	nextFrom, nextEnd := nextAt, rowsEnd // the Next list's lines
	if nextHead {
		nextFrom++
	}
	nextRows := len(items) // the queue index where the Next list ends
	if g.head >= 0 {
		nextEnd, nextRows = g.head, g.userEnd
	}

	row := func(i int) {
		l := g.lines[i]
		it := &items[l.qi]
		pos, size := l.qi-start, nextRows-start-1 // the list this row is in: Next
		switch {
		case l.cur:
			pos, size = 1, 1
		case g.head >= 0 && i > g.head:
			pos, size = l.qi-g.userEnd+1, len(items)-g.userEnd
		}
		p.queueRow(c, st, queueRowIn{
			item: it, qi: l.qi, start: start, userEnd: g.userEnd, n: len(items),
			num: l.qi - start, pos: pos, size: size,
			current: l.qi == index, playing: playing, upNext: l.qi == start+1,
			tab: l.qi == tabQi, dragged: dragging && dragFrom == l.qi,
			autoPlay: l.qi != index && it.Auto && autoPlay, withheld: withheldOK && it.Explicit,
			inView: inView,
		})
	}
	// section is the lines from..to as a list: only those built, the rest
	// held by a spacer of their height.
	section := func(key, name string, from, to int) {
		if from >= to {
			return
		}
		ui.Column(c.Key(key)).FillWidth().Role(ui.RoleList).Label(name).Children(func() {
			gap := -1 // the first line of the spacer being built
			flush := func(at int) {
				if gap >= 0 {
					end := g.total()
					if at < len(g.lines) {
						end = g.top(at)
					}
					ui.Box(c.Key("gap" + strconv.Itoa(at))).FillWidth().Height(end - g.top(gap)).Shrink(0)
					gap = -1
				}
			}
			for i := from; i < to; i++ {
				if !built(i) {
					if gap < 0 {
						gap = i
					}
					continue
				}
				flush(i)
				row(i)
			}
			flush(to)
		})
	}

	st.focusedNow = 0
	sc := ui.Scroll(c.Key("queue-scroll")).TrackScroll(&st.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c.Key("queue-content")).Padding(0, px, 12).Children(func() {
			body := ui.Column(c.Key("body")).FillWidth()
			body.Children(func() {
				if nowRow >= 0 {
					p.queueHead(c, "head-now", queueNowHeadH, i18n.T("Now playing"), false, nil)
					section("list-now", i18n.T("Now playing"), nowRow, nowRow+1)
				}
				if nextHead {
					var detail func()
					if !inView {
						// The pane's own foot says it on Now Playing.
						detail = func() {
							ui.Row(c.Key("detail")).MinWidth(0).Children(func() { p.queueSummary(c, nil) })
						}
					}
					p.queueHead(c, "head-next", queueHeadH, i18n.TC("the queue's heading over the songs that play after the one playing", "Next"), false, detail)
				}
				section("list", i18n.TC("the queue's heading over the songs that play after the one playing", "Next"), nextFrom, nextEnd)
				if g.head >= 0 {
					p.queueHead(c, "head", queueHeadH, headTitle, station != nil && station.AutoPlay, func() {
						label(c.Key("detail"), headDetail, "meta").TextColor(theme.C("ink-muted"))
					})
					section("list-station", headTitle, g.head+1, rowsEnd)
				}
				if rowsEnd < len(g.lines) {
					p.queueNote(c)
				}
				// A row that does not move (its own gaps) shows no line.
				if over && st.slotOn && (!dragging || (st.slot != dragFrom && st.slot != dragFrom+1)) {
					ui.Box(c.Key("drop-line")).Absolute().Left(8).Right(8).Top(g.slotTop(st.slot) - 1).Height(2).
						Radius(1).Background(theme.C("ink")).IgnoreInput(true)
				}
			})
			st.body = body.Bounds()
		})
	})
	if b := sc.Bounds(); b.H != st.paneH {
		st.paneH = b.H
		c.Invalidate()
	}

	// A row that had the focus and left the list (removed, played) hands it
	// to the row that takes its place; one that moved to the other list (an
	// AutoPlay song that plays now) keeps it.
	if st.focusedWas != 0 && st.focusedNow == 0 && st.want == 0 {
		at := indexOfUID(items, st.focusedWas)
		switch {
		case at < start:
			if qi := focusedRow(items, start, p.queue.focus); qi >= 0 {
				p.focusRow(st, items, qi)
				c.Invalidate()
			}
		case g.groupOf(at) != st.focusedGroup:
			// The row moved between the two lists (an AutoPlay song that plays
			// now): it is built under another parent, a new element, so it
			// takes the focus again.
			p.focusRow(st, items, at)
			c.Invalidate()
		}
	}
	st.focusedWas = st.focusedNow
	st.focusedGroup = 0
	if st.focusedNow != 0 {
		st.focusedGroup = g.groupOf(indexOfUID(items, st.focusedNow))
	}
	if st.want > 0 {
		st.tries++
		if st.tries > queueRetries {
			st.want = 0
		} else {
			st.scrollTo(st.want - 1)
			c.Invalidate()
		}
	}
}

// queueHead is the heading of a group of the queue: its name, and under it a
// line of detail where there is one. h is the line's height.
func (p *player) queueHead(c *ui.Context, key string, h float32, title string, infinity bool, detail func()) {
	ui.Column(c.Key(key)).FillWidth().Height(h).Shrink(0).Justify(ui.End).Padding(0, 8, 8).Children(func() {
		ui.Row(c.Key("title")).Gap(6).AlignItems(ui.Center).Role(ui.RoleHeading).Level(3).Label(title).Children(func() {
			if infinity {
				ui.Icon(c.Key("infinity"), theme.Icon("infinity")).FontSize(16).TextColor(theme.C("ink")).Shrink(0)
			}
			label(c.Key("text"), title, "ui").FontWeight(600).TextColor(theme.C("ink"))
		})
		if detail != nil {
			detail()
		}
	})
}

// queueNote is the line under the songs while AutoPlay is still on its way.
func (p *player) queueNote(c *ui.Context) {
	ui.Row(c.Key("note")).FillWidth().Height(queueNoteH).Shrink(0).AlignItems(ui.End).Gap(6).Padding(0, 8, 6).Children(func() {
		ui.Icon(c.Key("infinity"), theme.Icon("infinity")).FontSize(14).TextColor(theme.C("ink-muted")).Shrink(0)
		label(c.Key("text"), i18n.T("Then AutoPlay continues with similar music"), "meta").TextColor(theme.C("ink-muted"))
	})
}
