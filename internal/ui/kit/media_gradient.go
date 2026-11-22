package kit

import (
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

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
	rects  []fillBox
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
	s.rects = plainBoxes(s.layout.SelectionRects(ui.TextInputRange{Start: 0, End: len(utf16.Encode([]rune(text)))}))
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

// fillBox is one box of a run of text that a fill reaches: in the layout's
// coordinates, with how far the ink may overhang it to its left and right
// (an italic's slant), above (a word lifted past its line's top) and below
// (a descender of a face that has more than its line leaves).
type fillBox struct {
	r                         ui.Rect
	left, right, above, below float32
}

// plainBoxes are rects as fill boxes whose ink stays inside them.
func plainBoxes(rects []ui.Rect) []fillBox {
	out := make([]fillBox, len(rects))
	for i, r := range rects {
		out[i].r = r
	}
	return out
}

// mediaPaintFill draws layout again over itself in col, clipped to the part of
// boxes (the boxes of one run of text, in reading order, laid out at x, y)
// that progress, 0 to 1, has reached: solid up to p·(length + edge) − edge,
// then edge DIPs fading out over gradientSteps slices. It reports nothing;
// callers know whether progress moves.
func mediaPaintFill(p *ui.Painter, layout *ui.TextLayout, x, y float32, boxes []fillBox, progress, edge float32, rtl bool, col ui.Color) {
	mediaFill(p, boxes, x, y, progress, edge, rtl, col, func(c ui.Color) { p.TextLayout(layout, x, y, c) }, nil)
}

// mediaFill is mediaPaintFill for any drawing: draw paints the text in the
// colour it is given, once per slice, inside the slice's clip. place, when
// set, moves a slice (laid out at ox, oy) to where its text is drawn, and
// says false for one that holds none of it.
func mediaFill(p *ui.Painter, boxes []fillBox, ox, oy float32, progress, edge float32, rtl bool, col ui.Color, draw func(ui.Color), place func(ui.Rect) (ui.Rect, bool)) {
	if progress <= 0 || col.A == 0 {
		return
	}
	fill := func(r ui.Rect, c ui.Color) {
		if r.W <= 0 {
			return
		}
		if place != nil {
			var ok bool
			if r, ok = place(r); !ok {
				return
			}
		}
		p.Clip(r, 0, func() { draw(c) })
	}
	total := float32(0)
	for _, b := range boxes {
		total += b.r.W
	}
	// How far along the boxes the solid fill reaches; the edge runs on edge
	// DIPs beyond it (CSS: p·(100% + edge) − edge).
	solid := min(progress, 1)*(total+edge) - edge
	if progress >= 1 {
		solid = total
	}
	offset := float32(0)
	for _, b := range boxes {
		r := b.r
		r.X, r.Y = r.X+ox, r.Y+oy
		// The part of reading span from…to that falls in this box, grown
		// where it reaches the box's ends by the ink that overhangs them.
		span := func(from, to float32) ui.Rect {
			lo, hi := max(from-offset, 0), min(to-offset, r.W)
			if hi <= lo {
				return ui.Rect{}
			}
			x0 := r.X + lo
			if rtl {
				x0 = r.X + r.W - hi
			}
			x1 := x0 + hi - lo
			if !rtl && lo == 0 || rtl && hi == r.W {
				x0 -= b.left
			}
			if !rtl && hi == r.W || rtl && lo == 0 {
				x1 += b.right
			}
			return ui.Rect{X: x0, Y: r.Y - b.above, W: x1 - x0, H: r.H + b.above + b.below}
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
