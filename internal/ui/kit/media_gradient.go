package kit

import (
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// MyGo draws text in one colour; a gradient on text (the lyric sweep, the
// fill of a held note) is the text drawn again over itself in the fill
// colour, clipped to slices of its boxes: the part already reached at full
// strength, then gradientSteps slices of the soft edge, each a little
// fainter (the spike's lyricsweep.go; index.css's .lyric-word background-clip
// text with its --edge). The text is shaped once for its width, never per
// frame, and painted from the clock without building the view
// (Painter.AnimationFrame).

// gradientSteps is how many slices make the soft edge.
const gradientSteps = 8

// GradientOpts are the options of GradientText.
type GradientOpts struct {
	// Size, Weight and LineHeight are the font's, in DIPs (Weight 0: 400;
	// LineHeight 0: 1.3 × Size). The face is the theme's.
	Size, Weight, LineHeight float32
	// Width wraps the text at it; 0 shapes it on one line at its natural
	// width.
	Width float32
	// Base is the text's colour; Fill the colour of the part the gradient
	// has reached.
	Base, Fill ui.Color
	// Progress is how much of the text's length (every wrapped line's boxes
	// in reading order) the fill has reached, 0 to 1.
	Progress float32
	// Edge is the width in DIPs of the soft edge in front of the fill, which
	// runs on beyond it: the fill reaches the end of the text only at
	// Progress 1. 0 is half the font size (index.css's --edge: 0.5em).
	Edge float32
	// RTL fills from the right, as a right-to-left text is read.
	RTL bool
}

// gradientShape is the text shaped for a width: the layout, its size and the
// boxes of its lines in reading order.
type gradientShape struct {
	key    gradientKey
	layout *ui.TextLayout
	w, h   float32
	rects  []ui.Rect
}

type gradientKey struct {
	text                     string
	size, weight, lineHeight float32
	width                    float32
}

func (o GradientOpts) font() ui.Font {
	weight := int(o.Weight)
	if weight == 0 {
		weight = 400
	}
	lh := o.LineHeight
	if lh == 0 {
		lh = 1.3 * o.Size
	}
	return ui.Font{Family: theme.FontFamily, Size: o.Size, Weight: weight, LineHeight: lh / o.Size}
}

func shapeGradient(text string, o GradientOpts) *gradientShape {
	s := &gradientShape{key: gradientKey{text, o.Size, o.Weight, o.LineHeight, o.Width}}
	s.layout = ui.ShapeRichTextWith([]ui.Span{{Text: text}}, o.font(), ui.TextShape{Width: o.Width})
	s.w, s.h = s.layout.Size()
	s.rects = s.layout.SelectionRects(ui.TextInputRange{Start: 0, End: len(utf16.Encode([]rune(text)))})
	return s
}

// GradientText is text whose colour changes along its length: Base, with the
// fill colour reaching Progress of the way in reading order (left to right,
// right to left with RTL) behind a soft edge half an em wide. Painting only
// moves the clip, so a progress that changes every frame is cheap: ask for
// frames with the painter (GradientText itself builds once). It is
// decoration around the text: give the element a Label to name it for
// assistive technology, as the text is not a MyGo text element.
func GradientText(c *ui.Context, text string, o GradientOpts) ui.Element {
	box := ui.Box(c)
	st := ui.Local(box, "gradient", func() *gradientShape { return nil })
	if *st == nil || (*st).key != (gradientKey{text, o.Size, o.Weight, o.LineHeight, o.Width}) {
		*st = shapeGradient(text, o)
	}
	s := *st
	w := s.w
	if o.Width > 0 {
		w = o.Width
	}
	edge := o.Edge
	if edge == 0 {
		edge = o.Size / 2
	}
	box.Size(w, s.h).Draw(func(p *ui.Painter, r ui.Rect) {
		p.TextLayout(s.layout, r.X, r.Y, o.Base)
		mediaPaintFill(p, s.layout, r.X, r.Y, s.rects, o.Progress, edge, o.RTL, o.Fill)
	})
	return box
}

// mediaPaintFill draws layout again over itself in col, clipped to the part of
// rects (the boxes of one run of text, in reading order, laid out at x, y)
// that progress, 0 to 1, has reached: solid up to p·(length + edge) − edge,
// then edge DIPs fading out over gradientSteps slices. It reports nothing;
// callers know whether progress moves.
func mediaPaintFill(p *ui.Painter, layout *ui.TextLayout, x, y float32, rects []ui.Rect, progress, edge float32, rtl bool, col ui.Color) {
	if progress <= 0 || col.A == 0 {
		return
	}
	fill := func(r ui.Rect, c ui.Color) {
		if r.W > 0 {
			p.Clip(r, 0, func() { p.TextLayout(layout, x, y, c) })
		}
	}
	total := float32(0)
	for _, r := range rects {
		total += r.W
	}
	// How far along the boxes the solid fill reaches; the edge runs on edge
	// DIPs beyond it (CSS: p·(100% + edge) − edge).
	solid := min(progress, 1)*(total+edge) - edge
	if progress >= 1 {
		solid = total
	}
	offset := float32(0)
	for _, r := range rects {
		r.X, r.Y = r.X+x, r.Y+y
		// The part of reading span from…to that falls in this box.
		span := func(from, to float32) ui.Rect {
			lo, hi := max(from-offset, 0), min(to-offset, r.W)
			if hi <= lo {
				return ui.Rect{}
			}
			if rtl {
				return ui.Rect{X: r.X + r.W - hi, Y: r.Y, W: hi - lo, H: r.H}
			}
			return ui.Rect{X: r.X + lo, Y: r.Y, W: hi - lo, H: r.H}
		}
		fill(span(0, solid), col)
		if progress < 1 {
			for j := range gradientSteps {
				from := solid + edge*float32(j)/gradientSteps
				to := solid + edge*float32(j+1)/gradientSteps
				fill(span(max(0, from), to), mediaAlpha(col, 1-(float32(j)+0.5)/gradientSteps))
			}
		}
		offset += r.W
	}
}

// mediaAlpha is c with its alpha scaled by a (0 to 1).
func mediaAlpha(c ui.Color, a float32) ui.Color {
	c.A = uint8(float32(c.A)*min(max(a, 0), 1) + 0.5)
	return c
}

// SweepText is one word-timed text of a lyric line (its lead or its
// background vocals), shaped once for the sheet's width: each word fills
// with the sweep colour from its start to its end as it is sung, short
// words too, behind the soft edge (index.css's .lyric-word). Build one per
// line and width, keep it (shaping is the cost), and paint it from the
// clock in a Draw callback.
type SweepText struct {
	words  []lyrics.Word
	layout *ui.TextLayout
	width  float32
	h      float32
	size   float32
	rects  [][]ui.Rect // each word's boxes in the layout, wrapped words in several
}

// NewSweepText shapes words as one text of the font's size, weight and line
// height (DIPs) wrapped at width, in the theme's face.
func NewSweepText(words []lyrics.Word, size float32, weight int, lineHeight, width float32) *SweepText {
	return newSweepText(words, size, weight, false, lineHeight, width)
}

// NewSweepTextItalic is NewSweepText in italic: a duet's other voice.
func NewSweepTextItalic(words []lyrics.Word, size float32, weight int, lineHeight, width float32) *SweepText {
	return newSweepText(words, size, weight, true, lineHeight, width)
}

func newSweepText(words []lyrics.Word, size float32, weight int, italic bool, lineHeight, width float32) *SweepText {
	spans := make([]ui.Span, len(words))
	for i, w := range words {
		spans[i] = ui.Span{Text: w.Text}
	}
	font := ui.Font{Family: theme.FontFamily, Size: size, Weight: weight, Italic: italic, LineHeight: lineHeight / size}
	s := &SweepText{words: words, width: width, size: size}
	s.layout = ui.ShapeRichTextWith(spans, font, ui.TextShape{Width: width})
	_, s.h = s.layout.Size()
	// Each word's boxes, by UTF-16 offsets, without its trailing space.
	at := 0
	for _, w := range words {
		units := utf16.Encode([]rune(w.Text))
		trimmed := len(units)
		for trimmed > 0 && (units[trimmed-1] == ' ' || units[trimmed-1] == '\u00a0') {
			trimmed--
		}
		s.rects = append(s.rects, s.layout.SelectionRects(ui.TextInputRange{Start: at, End: at + trimmed}))
		at += len(units)
	}
	return s
}

// Width is the width the text was shaped for, Height its height in DIPs.
func (s *SweepText) Width() float32 { return s.width }

// Height is the shaped height in DIPs.
func (s *SweepText) Height() float32 { return s.h }

// Paint draws the text at (x, y) in base, then each word sung by time t over
// it in fill, a word being sung up to its soft edge; sung (0 to 1) scales the
// fill's strength, which is how a line fades in and out. It reports whether a
// word is being sung, which needs the next frame (Painter.AnimationFrame).
// rtl fills from the right.
func (s *SweepText) Paint(p *ui.Painter, x, y float32, base, fill ui.Color, sung float32, t time.Duration, rtl bool) (moving bool) {
	p.TextLayout(s.layout, x, y, base)
	if sung <= 0 {
		return false
	}
	hi := mediaAlpha(fill, sung)
	edge := s.size * 0.5
	for k, w := range s.words {
		if t < w.Begin {
			break // the words after it are later still
		}
		prog := float32(1)
		if t < w.End && w.End > w.Begin {
			prog, moving = float32(t-w.Begin)/float32(w.End-w.Begin), true
		}
		mediaPaintFill(p, s.layout, x, y, s.rects[k], prog, edge, rtl, hi)
	}
	return moving
}
