package discover

import (
	"math"
	"time"
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
	// leadShare is the most of the page's visible height the lead's art
	// takes, and leadMinHeight the least it is cut to by that.
	leadShare     = 0.45
	leadMinHeight = 280
	// leadCaptionGap is the room between the lead's progress and its
	// caption, whose rows are a 30 title, Apple's one line (leadLine, 2
	// below it) and an 18 byline.
	leadCaptionGap = 12
	leadLine       = 20
	// progressGap is the room between the lead's art and the rotation's
	// progress, a row of progressHeight segments, one a card, segmentGap
	// apart.
	progressGap    = 12
	progressHeight = 3
	segmentGap     = 4
	// leadChrome is the lead's height under its art: the progress and the
	// caption.
	leadChrome = progressGap + progressHeight + leadCaptionGap + 30 + 2 + leadLine + 18
	// moreReserve is the room the caption keeps at its end, beside the
	// pager's, for the card's menu button.
	moreReserve = 28
	// queueSize is how many cards the queue beside the lead shows: the next
	// ones in the rotation.
	queueSize = 4
	// queueBeside is the narrowest front column that sets the queue beside
	// the lead, in two rows of two taking queueShare of the column,
	// queueGap from the lead; narrower, the queue is a row under the lead,
	// queueNarrowGap apart, of four (two from queueTwoUnder down).
	queueBeside    = 900
	queueShare     = 0.40
	queueGap       = 24
	queueNarrowGap = 12
	queueTwoUnder  = 560
	// tileGapX and tileGapY are the gaps between the queue's tiles beside
	// the lead; tileCaption is a tile's height under its picture: an 8 gap,
	// a 20 title and an 18 byline.
	tileGapX    = 16
	tileGapY    = 20
	tileCaption = 8 + 20 + 18
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
	// lead is the lead's width and art its picture's height, which the
	// picture is requested at.
	lead, art float32
	// beside: the queue stands beside the lead; else it is a row under it.
	beside bool
	// columns is the queue's tiles in a row; tile a tile's width and
	// tileArt its picture's height (the hero card's shape).
	columns       int
	tile, tileArt float32
}

// layoutFor is the layout at a content width and the page's visible height
// (0 before it is known). From queueBeside the queue of the next cards
// stands beside the lead in two rows of two, and the lead's picture is as
// tall as makes the lead (picture, progress and caption) end with the
// queue's second row, kept between the banner's own 4:1 and the hero
// card's shape. Narrower, the lead takes the whole width, in the hero
// card's shape until that reaches leadMaxHeight or leadShare of the visible
// height, wider than it from there, with the queue in a row under it.
func layoutFor(width, view float32) frontLayout {
	w := floorf(max(width, 0))
	if w >= queueBeside {
		tile := floorf((w*queueShare - tileGapX) / 2)
		tileArt := floorf(tile / heroRatio)
		lead := w - (2*tile + tileGapX) - queueGap
		art := 2*(tileArt+tileCaption) + tileGapY - leadChrome
		art = floorf(min(max(art, lead/bannerRatio), lead/heroRatio, leadMaxHeight))
		return frontLayout{lead: lead, art: art, beside: true, columns: 2, tile: tile, tileArt: tileArt}
	}
	art := max(w/bannerRatio, min(w/heroRatio, leadMaxHeight))
	if view > 0 {
		art = min(art, max(leadMinHeight, view*leadShare))
	}
	columns := queueSize
	if w < queueTwoUnder {
		columns = 2
	}
	tile := floorf((w - float32(columns-1)*queueNarrowGap) / float32(columns))
	return frontLayout{lead: w, art: floorf(art), columns: columns, tile: tile, tileArt: floorf(tile / heroRatio)}
}

func floorf(v float32) float32 { return float32(math.Floor(float64(v))) }

// artAspect is the lead art box's width over its height, which its picture
// is requested at.
func (l frontLayout) artAspect() float32 {
	if l.art > 0 {
		return l.lead / l.art
	}
	return heroRatio
}

// progress is how far the lead is through its interval, 0 to 1, at now:
// 0 while nothing is waiting (the rotation holds or is paused).
func (r *rotation) progress(now time.Time) float32 {
	if !r.armed {
		return 0
	}
	left := r.due.Sub(now)
	return min(max(1-float32(left)/float32(rotateEvery), 0), 1)
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
// on offer, not paused by the user, with no pointer or keyboard focus inside
// the hero, the window visible and focused and the hero on screen. At the
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
