package discover

import (
	"math"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The front page's geometry, in DIPs.
const (
	// heroRatio is the hero card's shape on music.apple.com: its
	// "flowcaseLockup" requests the 4:1 subscriptionHero art cropped to
	// 1060×608 ("sr"). The lead keeps it while that is no taller than
	// leadMaxHeight.
	heroRatio = 1060.0 / 608
	// bannerRatio is the subscriptionHero art's own shape: the widest box
	// the lead may take, or Apple's crop would cut the art's top and bottom.
	bannerRatio = 4.0
	// leadMaxHeight is the tallest the lead's art gets. The lead spans the
	// whole column; past the width at which the hero shape would be taller
	// than this, the box widens towards the banner's own 4:1 instead (Apple
	// crops the centre of the banner to it, where its title and subject are),
	// so the front page never runs a screen tall.
	leadMaxHeight = 540
	// leadCaption is the lead's caption under its art: a 12 gap, a 30 title
	// line and an 18 line.
	leadCaption = 60
	// moreReserve is the room the caption keeps at its end, beside the
	// pager's, for the card's menu button.
	moreReserve = 28
	// listRow is a strip card's height; stripGap the space between cards.
	listRow  = 72
	stripGap = 8
	// stripPerPage is how many featured cards the strip shows at a time.
	stripPerPage = 4
	// listThumb is the strip thumbnail's slot: the item's square cover fills
	// it. A picture that is not square keeps its shape inside it (thumbBox).
	listThumb = 64
	// thumbSquare is how far a picture's shape may be from square (as a
	// ratio of width to height) and still fill the slot: Apple crops a
	// request to the box it is given, so a nearly square one loses nothing
	// that matters.
	thumbSquare = 1.15
	// stripWide is the narrowest front column that sets the strip's four
	// cards in one row (two rows of two below it).
	stripWide = 880
	// marginAt is the page width (the scroll pane's) from which a band's
	// heading stands in a margin column beside its shelves, marginWidth wide
	// with marginGap to the shelves; narrower, it stands above them.
	// contentsColumn is a band's column in the contents.
	contentsColumn = 216
	marginAt       = 1100
	marginWidth    = 200
	marginGap      = 32
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
	// lead is the lead's width, which its art is requested at; the caption and
	// the strip are as wide.
	lead float32
	// columns and rows are the strip's cards in a row and the rows of a page
	// (perPage = columns × rows); card is a card's width.
	columns, rows, perPage int
	card                   float32
	rowHeight              float32
}

// layoutFor is the layout at a content width. The lead takes the whole width,
// in Apple's hero shape until that reaches leadMaxHeight, wider than it from
// there (artHeight); the strip of featured cards under it shows four a page:
// in one row of four, or two rows of two in a narrow window.
func layoutFor(width float32) frontLayout {
	lead := float32(math.Floor(float64(max(width, 0))))
	columns, rows := stripPerPage, 1
	if lead < stripWide {
		columns, rows = 2, 2
	}
	// The strip reaches 8 past the lead's edges on each side (frontPage).
	card := float32(math.Floor(float64((lead + 16 - float32(columns-1)*stripGap) / float32(columns))))
	return frontLayout{lead: lead, columns: columns, rows: rows, perPage: columns * rows, card: card, rowHeight: listRow}
}

// artHeight is the lead art's height at its width: the hero shape, at most
// leadMaxHeight, and never wider than the banner itself.
func (l frontLayout) artHeight() float32 {
	return float32(math.Floor(float64(max(l.lead/bannerRatio, min(l.lead/heroRatio, leadMaxHeight)))))
}

// artAspect is the lead art box's width over its height, which its picture
// is requested at.
func (l frontLayout) artAspect() float32 {
	if h := l.artHeight(); h > 0 {
		return l.lead / h
	}
	return heroRatio
}

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
// height. The box is whole DIPs, so the picture (asked for at the box)
// lands on whole pixels and is not resampled.
func thumbBox(src string) (w, h float32) {
	switch a := kit.ArtAspect(src); {
	case a > thumbSquare:
		return listThumb, float32(math.Round(listThumb / heroRatio))
	case a < 1/thumbSquare:
		return float32(math.Round(float64(listThumb * a))), listThumb
	}
	return listThumb, listThumb
}
