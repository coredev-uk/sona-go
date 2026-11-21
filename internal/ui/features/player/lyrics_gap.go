package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// An instrumental gap (lyricsync.GapMinMs or more with no line sung, between
// two lines or before the first) is shown as three dots where a line would
// be: the dots are the sung line while the gap runs, and fill with the lit
// word's colour one by one as it passes, then clear before the next line.
// The sheet and Full-Screen Lyrics both set them; lyricsync works out when.

// gapState is where a gap's dots stand against the voice.
type gapState int

const (
	gapComing gapState = iota // the lines before are still sung: unlit, muted
	gapSung                   // the gap runs: the dots are the sung line
	gapPast                   // the next line has begun: cleared
)

// gapStateOf is the state of the dots before line i when line active is sung
// (-1: the voice rests) and the sheet is at line anchor (lyricsync.Sheet).
func gapStateOf(i, active, anchor int) gapState {
	switch {
	case i < anchor:
		return gapPast
	case i == anchor && active < 0:
		return gapSung
	case i == anchor:
		return gapPast
	}
	return gapComing
}

// gapTone is the colour of the dots not yet filled: ink while they are the
// sung line, as the words of a sung line are, muted before.
func gapTone(state gapState) ui.Color {
	if state == gapSung {
		return theme.C("ink")
	}
	return theme.C("ink-muted")
}

// gapDots is the size of the three dots: d across, space between them.
type gapDots struct{ d, space float32 }

func (g gapDots) width() float32 { return 3*g.d + 2*g.space }

// gapLabel names the dots for assistive technology.
func gapLabel() string { return i18n.T("Instrumental break") }

// gapDraw paints the dots of gap in a box, from the lyrics' clock: each dot
// in base, then filled over it in fill, a dot filling from the side the line
// after it is read from (right when it reads right to left). inset is kept
// clear at that side. The fill steps about 12 times a second, or whole dots
// as each third begins under reduced motion, and the painter is asked for the
// next step with a timer, never a loop of frames: nothing is asked while
// nothing plays or once the gap has passed.
func (p *player) gapDraw(songID string, gap lyricsync.Span, g gapDots, inset float32, right, reduced bool, base, fill ui.Color) func(*ui.Painter, ui.Rect) {
	return func(pt *ui.Painter, r ui.Rect) {
		ms := lyricsync.LyricsClock(p.env.Play.Position().Milliseconds(), lyricsync.Session().Get(songID))
		fills, next := lyricsync.GapFill(gap, ms, reduced)
		x0 := r.X + inset
		if right {
			x0 = r.X + r.W - inset - g.width()
		}
		y := r.Y + (r.H-g.d)/2
		for k, f := range fills {
			slot := k
			if right {
				slot = lyricsync.GapDots - 1 - k
			}
			dot := ui.Rect{X: x0 + float32(slot)*(g.d+g.space), Y: y, W: g.d, H: g.d}
			pt.Fill(dot, base, g.d/2)
			switch {
			case f >= 1:
				pt.Fill(dot, fill, g.d/2)
			case f > 0:
				part := dot
				part.W = g.d * f
				if right {
					part.X = dot.X + g.d - part.W
				}
				pt.Clip(part, 0, func() { pt.Fill(dot, fill, g.d/2) })
			}
		}
		if next != lyricsync.Never && p.env.Play.State().State == store.StatePlaying {
			pt.After(lyricsync.Dur(max(next-ms, 0) + 1))
		}
	}
}
