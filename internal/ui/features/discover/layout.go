package discover

import (
	"math"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The front page's geometry (v0.2.0's FrontPage.tsx), in DIPs.
const (
	// heroRatio is the hero card's shape on music.apple.com: its
	// "flowcaseLockup" requests the 4:1 subscriptionHero art cropped to
	// 1060×608 ("sr").
	heroRatio = 1060.0 / 608
	// leadCaption is the lead's caption under its art: an 8 gap, a 22 title
	// line and an 18 line.
	leadCaption = 48
	// pagerHeight is the list's pager row: an 8 gap and a 28 row.
	pagerHeight = 36
	// listRow is a list row's height under the lead, and the least beside it.
	listRow = 72
	// listThumb is the list thumbnail's slot: the item's square cover fills
	// it. A picture that is not square keeps its shape inside it (thumbBox).
	listThumb = 64
	// thumbSquare is how far a picture's shape may be from square (as a
	// ratio of width to height) and still fill the slot: Apple crops a
	// request to the box it is given, so a nearly square one loses nothing
	// that matters.
	thumbSquare = 1.15
	// splitAt is the narrowest content width that sets the list beside the
	// lead.
	splitAt = 880
	// rotateEvery is how long the lead stays before the next featured card
	// replaces it.
	rotateEvery = 7 * time.Second
	// leadFade is the cross-fade of a lead change (the lead-in and lead-out
	// animations).
	leadFade = 400 * time.Millisecond
	// leadSlide is how far the cards slide as they fade, 1rem.
	leadSlide = 16
)

// frontLayout is how the front page sets out at a width.
type frontLayout struct {
	// split: the list sits beside the lead (else under it).
	split bool
	// lead is the lead's width, which its art is requested at.
	lead float32
	// perPage is the list rows per page, perColumn per column, columns the
	// columns of the list.
	perPage, perColumn, columns int
	rowHeight                   float32
}

// layoutFor is the layout at a content width. Beside the lead (two thirds of
// the width), the list takes as many rows as fit the lead's height, stretched
// so its pager ends level with the lead's caption. Under the lead it runs in
// two columns of three (one under 560).
func layoutFor(width, gapX float32) frontLayout {
	if width >= splitAt {
		lead := float32(math.Round(float64((width - gapX) * 2 / 3)))
		room := lead/heroRatio + leadCaption - pagerHeight
		perPage := max(3, int(math.Floor(float64(room/listRow))))
		return frontLayout{split: true, lead: lead, perPage: perPage, perColumn: perPage, columns: 1, rowHeight: room / float32(perPage)}
	}
	columns := 1
	if width >= 560 {
		columns = 2
	}
	return frontLayout{lead: width, perPage: 3 * columns, perColumn: 3, columns: columns, rowHeight: listRow}
}

// artHeight is the lead art's height at its width.
func (l frontLayout) artHeight() float32 { return l.lead / heroRatio }

// rotation is the lead's timer: one deadline, no per-frame work (v0.2.0's
// useLeadRotation). While the lead may rotate (running) it waits
// rotateEvery from when it was armed, from the lead it was armed for: each
// change of the lead (the timer's or the user's) restarts the interval, so a
// chosen card is read for as long as any, and so does running holding again
// after a hold.
type rotation struct {
	armed bool
	at    int
	due   time.Time
	// last is the previous step: a gap longer than an interval (and a second
	// for the timer's lateness) since it means no frame ran (the window was
	// hidden, the computer asleep), and the interval starts over, as it does
	// when a hold ends.
	last time.Time
}

// restart starts a whole interval from now for the lead at: the lead has
// just changed, on the frame that changed it, so the next one does not wait
// for the frame after to count from.
func (r *rotation) restart(now time.Time, at int) {
	r.armed, r.at, r.due = true, at, now.Add(rotateEvery)
}

// step is called every frame. running says the lead may move on now: rotation
// on offer, not paused by the user, with no pointer or focus inside the
// hero, the window visible and focused and the hero on screen. At the
// deadline occupied (a menu is open, the page is covered) is asked: a tick
// that finds the page occupied waits a whole interval more. It reports
// whether the lead moves on now and, when it does not, how long until the
// deadline (0: nothing is waiting).
func (r *rotation) step(now time.Time, running bool, at int, occupied func() bool) (advance bool, wait time.Duration) {
	last := r.last
	r.last = now
	if !running {
		r.armed = false
		return false, 0
	}
	if r.armed && now.Sub(last) > rotateEvery+time.Second {
		r.armed = false
	}
	if !r.armed || r.at != at {
		r.armed, r.at, r.due = true, at, now.Add(rotateEvery)
	}
	if now.Before(r.due) {
		return false, r.due.Sub(now)
	}
	if occupied() {
		r.due = now.Add(rotateEvery)
		return false, rotateEvery
	}
	return true, 0
}

// thumbBox is the box the picture at src fills in the list's listThumb slot,
// from the shape its URL names. A square cover fills the slot. A wide picture
// (a station's key art, a card that has only its banner) is shown as the lead
// shows it (Apple's centre crop at the hero card's shape), across the slot's
// width and centred, where filling the slot would cut a square out of the
// middle of a 4:1 banner and show that, enlarged. A tall one fits the slot's
// height.
func thumbBox(src string) (w, h float32) {
	switch a := kit.ArtAspect(src); {
	case a > thumbSquare:
		return listThumb, listThumb / heroRatio
	case a < 1/thumbSquare:
		return listThumb * a, listThumb
	}
	return listThumb, listThumb
}
