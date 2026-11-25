package player

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
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

// gapTone is the colour of a dot not yet filled: the ink of a line still to
// come in the view (--ink-muted in the sheet and in Full-Screen Lyrics), so
// that the dots fill to --amber-ink as a word lights, and are never brighter
// than the lines they stand among.
func gapTone() ui.Color { return theme.C("ink-muted") }

// The dots are sized from the font of the line they lead to: each gapDotEm
// across, gapSpaceEm apart, so that they read as a line of the lyrics.
const (
	gapDotEm   = 0.6
	gapSpaceEm = 0.25
)

// gapDots is the size of the three dots: d across, space between them.
type gapDots struct{ d, space float32 }

// dotsFor is the dots of a line set at size.
func dotsFor(size float32) gapDots {
	return gapDots{d: max(round32(size*gapDotEm), 6), space: max(round32(size*gapSpaceEm), 3)}
}

func (g gapDots) width() float32 { return 3*g.d + 2*g.space }

// gapLabel names the dots for assistive technology.
func gapLabel() string { return i18n.T("Instrumental break") }

// While the gap runs the dots ride a slow wave that travels in the reading
// direction, so the break reads as music going on rather than a stall: each
// dot rises and falls gapBob of its width on a gapWaveMs period, a sixth of
// a period behind the dot before it, the wave growing in over gapWaveInMs.
// Before the next line the wave settles while the dots swell (gapSwell, over
// gapSwellMs, the house curve), then they gather into the middle dot's place
// and shrink away, gathering speed (eased in), gone the moment the line
// begins (gapExitMs in all). Under reduced motion the dots stand still and
// clear when the line begins, as every other lyric motion does.
const (
	gapWaveMs   = 2400
	gapWaveInMs = 600
	gapBob      = 0.22
	gapSwellMs  = 180
	gapExitMs   = 520
	gapSwell    = 0.15
)

// houseEase is the house curve, cubic-bezier(0.16, 1, 0.3, 1) (docs/design.md,
// "Motion").
var houseEase = ui.CubicBezier(0.16, 1, 0.3, 1)

// dotPose is where a gap's dot stands at a moment: dy its rise (in dot
// widths, up positive), gather how far it has moved to the middle dot's place
// (0 to 1), and scale its size (0 is gone).
type dotPose struct{ dy, gather, scale float32 }

// gapPose is the pose of the k-th dot in reading order of gap at ms on the
// lyrics' clock, and whether the dots move then (and so want frames). Before
// the gap, and under reduced motion while it runs, the dots stand still at
// full size; once it has passed they are gone, also in the frame or two
// before the sheet stops drawing them.
func gapPose(gap lyricsync.Span, ms int64, k int, reduced bool) (dotPose, bool) {
	switch {
	case ms >= gap.EndMs:
		return dotPose{}, false
	case reduced || ms < gap.BeginMs:
		return dotPose{scale: 1}, false
	}
	e, left := float64(ms-gap.BeginMs), float64(gap.EndMs-ms)
	var swell, collapse float32
	if left < gapExitMs {
		swell = houseEase(float32(min((gapExitMs-left)/gapSwellMs, 1)))
		collapse = ui.EaseIn(float32(max(min((gapExitMs-gapSwellMs-left)/(gapExitMs-gapSwellMs), 1), 0)))
	}
	amp := gapBob * houseEase(float32(min(e/gapWaveInMs, 1))) * (1 - swell)
	phase := 2*math.Pi*e/gapWaveMs - float64(k)*math.Pi/3
	return dotPose{
		dy:     amp * float32(math.Sin(phase)),
		gather: collapse,
		scale:  (1 + gapSwell*swell) * (1 - collapse),
	}, true
}

// gapDraw paints the dots of gap in a box, from the lyrics' clock: each dot
// in base, then filled over it in fill, a dot filling from the side the line
// after it is read from (right when it reads right to left). inset is kept
// clear at that side. The fill steps about 12 times a second, or whole dots
// as each third begins under reduced motion. While the gap runs and a song
// plays, the dots move (gapPose) and the painter is asked for the next frame,
// painting only the dots, as the playing row's equaliser does; otherwise it
// is asked for the next fill step with a timer. Nothing is asked while
// nothing plays or once the gap has passed. A dot moves by a transform, never
// by moving its rectangle, whose edges would snap to whole pixels and step.
func (p *player) gapDraw(songID string, gap lyricsync.Span, g gapDots, inset float32, right, reduced bool, base, fill ui.Color) func(*ui.Painter, ui.Rect) {
	return func(pt *ui.Painter, r ui.Rect) {
		ms := lyricsync.LyricsClock(p.env.Play.Position().Milliseconds(), lyricsync.Session().Get(songID))
		fills, next := lyricsync.GapFill(gap, ms, reduced)
		x0 := r.X + inset
		if right {
			x0 = r.X + r.W - inset - g.width()
		}
		y := r.Y + (r.H-g.d)/2
		middle := x0 + g.d + g.space // the middle dot's place, where they gather
		moving := false
		for k, f := range fills {
			pose, moves := gapPose(gap, ms, k, reduced)
			moving = moving || moves
			if pose.scale <= 0 {
				continue
			}
			slot := k
			if right {
				slot = lyricsync.GapDots - 1 - k
			}
			dot := ui.Rect{X: x0 + float32(slot)*(g.d+g.space), Y: y, W: g.d, H: g.d}
			paint := func() {
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
			if !moves {
				paint()
				continue
			}
			t := ui.Scaling(pose.scale, pose.scale).Then(ui.Translation((middle-dot.X)*pose.gather, -pose.dy*g.d))
			pt.Transform(t, dot.X+g.d/2, dot.Y+g.d/2, paint)
		}
		if p.env.Play.State().State != store.StatePlaying {
			return
		}
		if moving {
			kit.AnimationFrames(pt)
			return
		}
		if next != lyricsync.Never {
			pt.After(lyricsync.Dur(max(next-ms, 0) + 1))
		}
	}
}
