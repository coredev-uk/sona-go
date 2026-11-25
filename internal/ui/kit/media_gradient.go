package kit

import (
	"math"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// MyGo draws text in one colour; a gradient on text (the lyric sweep, the
// fill of a held note) is the text drawn several times, each clipped to a
// stretch of its boxes in one colour: the part already reached in the fill
// colour, then gradientSteps slices of the soft edge, each the fill a little
// fainter mixed into the base, then the base for the rest (the spike's
// lyricsweep.go; index.css's .lyric-word background-clip text with its
// --edge). Every pixel is painted once, in one colour: drawing the fill over
// text already drawn in the base would leave the base's colour along every
// glyph edge, as an edge pixel of coverage c keeps (1−c)·c of what is under
// the fill (issue #385). The text is shaped once for its width, never per
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

// gradientShape is the text shaped for a width: the layout, its size, its
// lines and the boxes of its lines in reading order, with the scratch its
// painting reuses.
type gradientShape struct {
	key       gradientKey
	layout    *ui.TextLayout
	w, h      float32
	lines     []ui.TextLine
	rects     []fillBox
	segs, row []mediaSeg
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
	s.lines = s.layout.Lines()
	s.rects = plainBoxes(s.layout.SelectionRects(ui.TextInputRange{Start: 0, End: len(utf16.Encode([]rune(text)))}), s.lines)
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
		if o.Progress <= 0 || o.Fill.A == 0 {
			p.TextLayout(s.layout, r.X, r.Y, o.Base)
			return
		}
		s.segs = mediaSegments(s.segs[:0], s.rects, r.X, r.Y, o.Progress, edge, o.RTL, o.Base, o.Fill)
		s.row = mediaBands(p, s.layout, s.lines, s.segs, s.row, r.X, r.Y, max(w, s.w), s.h, o.Size, o.Base)
	})
	return box
}

// fillBox is one box of a run of text that a fill reaches: in the layout's
// coordinates, with how far the ink may overhang it to its left and right
// (an italic's slant), above (a word lifted past its line's top) and below
// (a descender of a face that has more than its line leaves), and the
// layout's line it is on.
type fillBox struct {
	r                         ui.Rect
	left, right, above, below float32
	line                      int
}

// plainBoxes are rects as fill boxes whose ink stays inside them, each on
// its line of lines.
func plainBoxes(rects []ui.Rect, lines []ui.TextLine) []fillBox {
	out := make([]fillBox, len(rects))
	for i, r := range rects {
		out[i].r = r
		out[i].line = lineAt(lines, r.Y)
	}
	return out
}

// lineAt is the index of the layout's line that starts nearest y.
func lineAt(lines []ui.TextLine, y float32) (at int) {
	best := float32(math.MaxFloat32)
	for i, ln := range lines {
		if d := float32(math.Abs(float64(ln.Rect.Y - y))); d < best {
			best, at = d, i
		}
	}
	return at
}

// mediaSeg is a stretch of one box of a run of text that is painted in one
// colour: its rect, laid out (grown by the box's overhangs where it reaches
// the box's ends, and as high and as low as the box's), the colour, and the
// box's index and line.
type mediaSeg struct {
	r         ui.Rect
	c         ui.Color
	box, line int
}

// mediaSegments appends to segs the stretches of boxes (the boxes of one run
// of text, in reading order, laid out at ox, oy) in the colours a fill that
// progress, 0 to 1, has reached paints them: fill up to the front, p·(length
// + edge) − edge, then edge DIPs of fill fading into base, then base. The
// fade is gradientSteps slices that lie where they are, edge/gradientSteps
// apart along the run, and take the colour of the fade at their middle: a
// stretch is a clip, whose edges snap to whole device pixels, so slices that
// travelled with the front would step a pixel at a time across a slow word,
// where slices that stay and change colour move the front by the part of a
// pixel each frame asks. Each box is covered once, from its left overhang to
// its right, by its stretches in order from left to right, neighbours meeting
// exactly. A fill with alpha (a line fading in) is mixed into the base, never
// drawn over it.
func mediaSegments(segs []mediaSeg, boxes []fillBox, ox, oy float32, progress, edge float32, rtl bool, base, fill ui.Color) []mediaSeg {
	const far = math.MaxFloat32
	if fill.A == 0 {
		progress = 0
	}
	total := float32(0)
	for _, b := range boxes {
		total += b.r.W
	}
	// How far along the boxes the solid fill reaches; the edge runs on edge
	// DIPs beyond it (CSS: p·(100% + edge) − edge).
	solid := min(progress, 1)*(total+edge) - edge
	offset := float32(0)
	for k, b := range boxes {
		r := b.r
		r.X, r.Y = r.X+ox, r.Y+oy
		first := len(segs)
		// The part of the reading span from…to that falls in this box, grown
		// where it reaches the box's ends by the ink that overhangs them.
		add := func(from, to float32, c ui.Color) {
			lo, hi := max(from-offset, 0), min(to-offset, r.W)
			if hi <= lo {
				return
			}
			x0, x1 := r.X+lo, r.X+hi
			if rtl {
				x0, x1 = r.X+r.W-hi, r.X+r.W-lo
			}
			if !rtl && lo == 0 || rtl && hi == r.W {
				x0 -= b.left
			}
			if !rtl && hi == r.W || rtl && lo == 0 {
				x1 += b.right
			}
			segs = append(segs, mediaSeg{r: spanRect(x0, x1, r.Y-b.above, r.Y+r.H+b.below), c: c, box: k, line: b.line})
		}
		switch {
		case progress <= 0:
			add(-far, far, base)
		case progress >= 1:
			add(-far, far, mediaTint(base, fill, 1))
		default:
			step := edge / gradientSteps
			c := int(math.Floor(float64(solid/step) + 0.5)) // the first slice whose middle is past the front
			add(-far, float32(c)*step, mediaTint(base, fill, 1))
			for ; ; c++ {
				v := (float32(c)+0.5)*step - solid // how far its middle is into the fade, in DIPs
				if v >= edge {
					break
				}
				add(float32(c)*step, float32(c+1)*step, mediaTint(base, fill, 1-v/edge))
			}
			add(float32(c)*step, far, base)
		}
		if rtl { // read from the right: left to right is the other way round
			for i, j := first, len(segs)-1; i < j; i, j = i+1, j-1 {
				segs[i], segs[j] = segs[j], segs[i]
			}
		}
		offset += b.r.W
	}
	return segs
}

// boxSegs are the stretches of segs (mediaSegments') of box k.
func boxSegs(segs []mediaSeg, k int) []mediaSeg {
	from := 0
	for from < len(segs) && segs[from].box != k {
		from++
	}
	to := from
	for to < len(segs) && segs[to].box == k {
		to++
	}
	return segs[from:to]
}

// mediaRow paints, from x0 to x1 and top to bottom, with draw (the text in
// the colour it is given) in the colours of segs (left to right, apart or
// meeting) and in base where none lies: each stretch is one clip, and
// neighbours of one colour share theirs, so that every pixel is painted once.
func mediaRow(p *ui.Painter, segs []mediaSeg, x0, x1, top, bottom float32, base ui.Color, draw func(ui.Color)) {
	from, col, at := x0, base, x0
	flush := func(to float32) {
		if to > from {
			p.Clip(spanRect(from, to, top, bottom), 0, func() { draw(col) })
		}
	}
	put := func(x float32, c ui.Color) {
		if c != col {
			flush(x)
			from, col = x, c
		}
	}
	for _, s := range segs {
		a, b := max(s.r.X, at), min(s.r.X+s.r.W, x1)
		if b <= a {
			continue
		}
		if a > at {
			put(at, base)
		}
		put(a, s.c)
		at = b
	}
	if at < x1 {
		put(at, base)
	}
	flush(x1)
}

// mediaBands paints layout at (x, y), w by h, in the colours of segs (which
// may be for some of its boxes only) and in base elsewhere, every pixel once:
// each of its lines is a band, from its top to the next line's (the first
// and the last run on margin DIPs), painted as a row of the stretches on that
// line. row is scratch for the rows; mediaBands returns it to be used again.
func mediaBands(p *ui.Painter, layout *ui.TextLayout, lines []ui.TextLine, segs, row []mediaSeg, x, y, w, h, margin float32, base ui.Color) []mediaSeg {
	if len(lines) == 0 {
		p.TextLayout(layout, x, y, base)
		return row
	}
	draw := func(c ui.Color) { p.TextLayout(layout, x, y, c) }
	for i := range lines {
		top := y + lines[i].Rect.Y
		if i == 0 {
			top = y - margin
		}
		bottom := y + h + margin
		if i+1 < len(lines) {
			bottom = y + lines[i+1].Rect.Y
		}
		row = row[:0]
		for _, s := range segs {
			if s.line != i {
				continue
			}
			// In order from the left (a line of words read right to left
			// comes the other way round); a line holds a few.
			row = append(row, s)
			for j := len(row) - 1; j > 0 && row[j].r.X < row[j-1].r.X; j-- {
				row[j], row[j-1] = row[j-1], row[j]
			}
		}
		mediaRow(p, row, x-margin, x+w+margin, top, bottom, base, draw)
	}
	return row
}

// spanRect is the rect from x0 to x1 and y0 to y1 whose X+W and Y+H come to
// x1 and y1 exactly, as far as float32 allows, so that two clips that meet
// there snap their edges to the same device pixel.
func spanRect(x0, x1, y0, y1 float32) ui.Rect {
	return ui.Rect{X: x0, Y: y0, W: spanTo(x0, x1), H: spanTo(y0, y1)}
}

func spanTo(a, b float32) float32 {
	w := b - a
	for range 4 {
		switch {
		case a+w < b:
			w = math.Nextafter32(w, math.MaxFloat32)
		case a+w > b:
			w = math.Nextafter32(w, -math.MaxFloat32)
		default:
			return w
		}
	}
	return w
}

// mediaTint is fill at strength a (0 to 1, times its own alpha) composited
// over base: what the fill drawn over the base would show where the glyphs
// cover the pixel, painted in one pass instead.
func mediaTint(base, fill ui.Color, a float32) ui.Color {
	af := min(max(a, 0), 1) * float32(fill.A) / 255
	ab := float32(base.A) / 255
	ao := af + ab*(1-af)
	if ao <= 0 {
		return ui.Transparent
	}
	c := base.Mix(fill, af/ao)
	c.A = uint8(ao*255 + 0.5)
	return c
}

// mediaAlpha is c with its alpha scaled by a (0 to 1).
func mediaAlpha(c ui.Color, a float32) ui.Color {
	c.A = uint8(float32(c.A)*min(max(a, 0), 1) + 0.5)
	return c
}
