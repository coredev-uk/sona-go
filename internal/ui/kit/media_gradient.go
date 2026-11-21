package kit

import (
	"math"
	"strings"
	"time"
	"unicode"
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

// SweepText is one word-timed text of a lyric line (its lead or its
// background vocals), shaped once for the sheet's width: each word fills
// with the sweep colour from its start to its end as it is sung, short
// words too, behind the soft edge (index.css's .lyric-word), and moves as
// it is: it lifts a little (liftEm, twice that for background vocals) and,
// held long enough, swells letter by letter with a glow (media_wordmotion.go).
// Build one per line and width, keep it (shaping is the cost), and paint it
// from the clock in a Draw callback.
type SweepText struct {
	words  []sweepWord
	layout *ui.TextLayout
	font   ui.Font
	width  float32
	h      float32
	size   float32

	begin  time.Duration // when the first word begins
	maxDur time.Duration // the longest lift
	empEnd time.Duration // when the last swell has run
}

// sweepWord is one timed word: its boxes in the layout, wrapped words in
// several, and how it moves.
type sweepWord struct {
	text       string
	begin, end time.Duration
	dur        time.Duration // the lift's: max(1 s, end − begin)
	up         float32       // how far the word rests raised, in em
	boxes      []fillBox
	emp        *sweepSwell // nil for a word that only lifts
}

// sweepSwell is a held word's swell: its letters and where they stand.
type sweepSwell struct {
	swell
	run      time.Duration // how long the whole swell takes
	baseline float32       // of the word's line, in the layout
	centre   float32       // y of the middle of its box, which the letters scale about
	letters  []sweepLetter
}

// sweepLetter is one letter (a grapheme) of a swelling word: its place in
// the layout and its glyphs at each size it is drawn at (index k: 1 +
// swellStep·k times the text's size).
type sweepLetter struct {
	x, w   float32
	glyphs [swellSteps + 1][]ui.Glyph
}

// SweepClock is where the voice stands against a SweepText at one frame.
type SweepClock struct {
	// T is the word clock: the song's position under the lyrics' offset.
	T time.Duration
	// Sung (0 to 1) scales the fill's strength, which is how a line fades
	// in and out; 0 paints the text in its base colour alone.
	Sung float32
	// Until is when the line stops being sung (NoUntil: not before the song
	// ends). The words that lifted settle back from there, and the line is
	// still, with nothing asked of the next frame, once they have.
	Until time.Duration
	// Still is reduced motion: the words light whole and nothing lifts,
	// swells or glows.
	Still bool
}

// NoUntil is the Until of a line that is sung to the end of the song.
const NoUntil = time.Duration(math.MaxInt64)

// NewSweepText shapes words as one text of the font's size, weight and line
// height (DIPs) wrapped at width, in the theme's face. lang is the words'
// language (a BCP 47 tag, "" the app's): it picks the glyphs of Han
// characters and where lines may break, so Korean words stay whole as in a
// Text of that Lang.
func NewSweepText(words []lyrics.Word, lang string, size float32, weight int, lineHeight, width float32) *SweepText {
	return newSweepText(words, lang, size, weight, false, lineHeight, width)
}

// NewSweepTextItalic is NewSweepText in italic: a duet's other voice.
func NewSweepTextItalic(words []lyrics.Word, lang string, size float32, weight int, lineHeight, width float32) *SweepText {
	return newSweepText(words, lang, size, weight, true, lineHeight, width)
}

func newSweepText(words []lyrics.Word, lang string, size float32, weight int, italic bool, lineHeight, width float32) *SweepText {
	spans := make([]ui.Span, len(words))
	for i, w := range words {
		spans[i] = ui.Span{Text: w.Text}
	}
	font := ui.Font{Family: theme.FontFamily, Size: size, Weight: weight, Italic: italic, Lang: lang, LineHeight: lineHeight / size}
	s := &SweepText{words: make([]sweepWord, len(words)), width: width, size: size, font: font}
	s.layout = ui.ShapeRichTextWith(spans, font, ui.TextShape{Width: width})
	_, s.h = s.layout.Size()
	// Each word's boxes, by UTF-16 offsets, without its trailing space.
	lines := s.layout.Lines()
	at := 0
	last := -1 // the last word with something to show
	for i, w := range words {
		if strings.TrimSpace(w.Text) != "" {
			last = i
		}
	}
	for i, w := range words {
		units := utf16.Encode([]rune(w.Text))
		trimmed := len(units)
		for trimmed > 0 && (units[trimmed-1] == ' ' || units[trimmed-1] == '\u00a0') {
			trimmed--
		}
		sw := &s.words[i]
		sw.text, sw.begin, sw.end = w.Text, w.Begin, w.End
		sw.dur = max(liftMin, w.End-w.Begin)
		sw.up = liftEm
		if w.Background {
			sw.up = 2 * liftEm
		}
		sw.boxes = plainBoxes(s.layout.SelectionRects(ui.TextInputRange{Start: at, End: at + trimmed}))
		for k := range sw.boxes {
			sw.boxes[k].above, sw.boxes[k].below = sw.up*size, sw.up*size
		}
		if emphasizes(w.Text, w.End-w.Begin) && len(sw.boxes) == 1 && !hasRTL(w.Text) {
			sw.emp = s.swellFor(sw, at, w.End-w.Begin, i == last, w.Background, lines)
		}
		at += len(units)
	}
	s.padBoxes()
	for i := range s.words {
		w := &s.words[i]
		if i == 0 || w.begin < s.begin {
			s.begin = w.begin
		}
		settle := w.dur
		if w.emp != nil {
			settle = liftMin // a swollen word settles over a second
			s.empEnd = max(s.empEnd, w.begin+w.emp.run+settle)
		}
		s.maxDur = max(s.maxDur, settle)
	}
	return s
}

// swellFor is word w's swell, nil where its letters cannot be told apart in
// the layout. at is the UTF-16 offset the word starts at.
func (s *SweepText) swellFor(w *sweepWord, at int, dur time.Duration, last, bg bool, lines []ui.TextLine) *sweepSwell {
	var letters []sweepLetter
	rest := strings.TrimSpace(w.text)
	at += len(utf16.Encode([]rune(w.text[:len(w.text)-len(strings.TrimLeftFunc(w.text, unicode.IsSpace))]))) // past its leading space
	for rest != "" {
		g := firstGrapheme(rest)
		rest = rest[len(g):]
		n := len(utf16.Encode([]rune(g)))
		rects := s.layout.SelectionRects(ui.TextInputRange{Start: at, End: at + n})
		at += n
		if len(rects) != 1 || rects[0].W <= 0 {
			return nil
		}
		l := sweepLetter{x: rects[0].X, w: rects[0].W}
		for k := range l.glyphs {
			f := s.font
			f.Size = s.size * (1 + swellStep*float32(k))
			l.glyphs[k] = ui.Shape(g, f)
		}
		letters = append(letters, l)
	}
	if len(letters) == 0 {
		return nil
	}
	box := w.boxes[0].r
	e := &sweepSwell{swell: newSwell(dur, len(letters), last, bg), letters: letters, centre: box.Y + box.H/2}
	e.run = e.total()
	// The baseline of the line the word is on.
	best := float32(math.MaxFloat32)
	for _, ln := range lines {
		if d := float32(math.Abs(float64(ln.Rect.Y - box.Y))); d < best {
			best, e.baseline = d, ln.Baseline
		}
	}
	return e
}

// padBoxes works out how far each box's ink may overhang it: 0.15 em either
// side (an italic's slant), but no further than 60% of the way to the next
// box on its line, so that words set close together (syllables of one word)
// never draw each other's ink.
func (s *SweepText) padBoxes() {
	over := s.size * 0.15
	for i := range s.words {
		for k := range s.words[i].boxes {
			b := &s.words[i].boxes[k]
			b.left, b.right = over, over
			for j := range s.words {
				for m := range s.words[j].boxes {
					o := s.words[j].boxes[m].r
					if (j == i && m == k) || o.W <= 0 || o.Y+o.H <= b.r.Y+1 || o.Y >= b.r.Y+b.r.H-1 {
						continue // itself, or on another line
					}
					switch {
					case o.X+o.W <= b.r.X+0.5:
						b.left = min(b.left, max(b.r.X-(o.X+o.W), 0)*0.6)
					case o.X >= b.r.X+b.r.W-0.5:
						b.right = min(b.right, max(o.X-(b.r.X+b.r.W), 0)*0.6)
					}
				}
			}
		}
	}
}

// hasRTL reports a letter of a script read right to left, whose letters join
// and so are not swollen one by one.
func hasRTL(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Hebrew, unicode.Arabic, unicode.Syriac, unicode.Thaana) {
			return true
		}
	}
	return false
}

// Width is the width the text was shaped for, Height its height in DIPs.
func (s *SweepText) Width() float32 { return s.width }

// Height is the shaped height in DIPs.
func (s *SweepText) Height() float32 { return s.h }

// state is where word w stands at t, its line sung until until: how far it
// is raised (em, up), whether its letters are mid-swell and how far into it
// (swelling, f), and whether it is still moving, which asks for the next
// frame.
func (w *sweepWord) state(t, until time.Duration) (lift float32, swelling bool, f time.Duration, moving bool) {
	if w.emp == nil {
		s := wordClock(t, w.begin, until, w.dur)
		moving = t <= until && s >= 0 && s < w.dur || t > until && s > 0
		return w.up * easeOut.at(float32(s)/float32(w.dur)), false, 0, moving
	}
	// A swell runs to its end whether or not the line is still sung: the
	// letters finish what they began (AMLL does not play it back). The word
	// rests raised, and settles back, over a second, once the line has
	// stopped and the swell is over.
	f = t - w.begin
	switch run := w.emp.run; {
	case f < 0:
		return 0, false, f, false
	case f < run:
		return w.up, true, f, true
	}
	start := max(until, w.begin+w.emp.run)
	if t <= start {
		return w.up, false, f, false
	}
	s := liftMin - (t - start)
	return w.up * easeOut.at(float32(s)/float32(liftMin)), false, f, s > 0
}

// lifts reports whether any word may be away from its place at c: from the
// first word's begin until the last has settled. Outside it the text is
// painted whole.
func (s *SweepText) lifts(c SweepClock) bool {
	switch {
	case c.Still || c.T < s.begin:
		return false
	case c.T <= c.Until:
		return true
	}
	return c.T-c.Until < s.maxDur || c.T < s.empEnd
}

// Paint draws the text at (x, y) in base, then each word sung by clock.T over
// it in fill, a word being sung up to its soft edge; clock.Sung (0 to 1)
// scales the fill's strength, which is how a line fades in and out. Each word
// lifts as it begins, and a held one swells; the fill moves with its word.
// It reports whether a word is moving, which needs the next frame
// (Painter.AnimationFrame): filling, lifting or settling. rtl fills from the
// right.
func (s *SweepText) Paint(p *ui.Painter, x, y float32, base, fill ui.Color, clock SweepClock, rtl bool) (moving bool) {
	hi := mediaAlpha(fill, clock.Sung)
	edge := s.size * 0.5
	if !s.lifts(clock) {
		p.TextLayout(s.layout, x, y, base)
		if clock.Sung <= 0 {
			return false
		}
		for k := range s.words {
			w := &s.words[k]
			if clock.T < w.begin {
				break // the words after it are later still
			}
			prog, sweeping := w.progress(clock.T)
			moving = moving || sweeping
			mediaPaintFill(p, s.layout, x, y, w.boxes, prog, edge, rtl, hi)
		}
		return moving
	}
	for k := range s.words {
		w := &s.words[k]
		lift, swelling, f, mv := w.state(clock.T, clock.Until)
		moving = moving || mv
		prog, sweeping := float32(0), false
		if clock.Sung > 0 && clock.T >= w.begin {
			prog, sweeping = w.progress(clock.T)
			moving = moving || sweeping
		}
		if swelling {
			s.paintLetters(p, x, y, w, f, base, hi, prog, edge, rtl)
			continue
		}
		dy := -lift * s.size
		draw := func(c ui.Color) { p.TextLayout(s.layout, x, y+dy, c) }
		for _, b := range w.boxes {
			p.Clip(ui.Rect{X: x + b.r.X - b.left, Y: y + b.r.Y - b.above, W: b.r.W + b.left + b.right, H: b.r.H + b.above + b.below}, 0, func() { draw(base) })
		}
		mediaFill(p, w.boxes, x, y, prog, edge, rtl, hi, draw, nil)
	}
	return moving
}

// progress is how far the fill has gone through the word at t, and whether it
// still goes.
func (w *sweepWord) progress(t time.Duration) (float32, bool) {
	if t < w.end && w.end > w.begin {
		return float32(t-w.begin) / float32(w.end-w.begin), true
	}
	return 1, false
}

// paintLetters draws a swelling word letter by letter, f into its swell: each
// at its own size (shaped at the nearest step, so it stays sharp), about its
// own centre, spread from the word's, with a glow of the fill's colour
// behind it, the fill of the word's sweep passing over it as it moves.
func (s *SweepText) paintLetters(p *ui.Painter, x, y float32, w *sweepWord, f time.Duration, base, fill ui.Color, prog, edge float32, rtl bool) {
	e := w.emp
	size := s.size
	for i := range e.letters {
		l := &e.letters[i]
		pose := e.pose(i, f)
		k := min(max(int((pose.scale-1)/swellStep+0.5), 0), swellSteps)
		sq := 1 + swellStep*float32(k)
		c0 := x + l.x + l.w/2 // where the letter's centre is laid out
		cx := c0 + pose.dx*size
		gx := cx - l.w*sq/2
		cy := y + e.centre
		by := cy + (y+e.baseline-cy)*sq + pose.dy*size // the baseline, scaled about the box's middle
		glyphs := l.glyphs[k]
		if fill.A > 0 && pose.glow > 0.01 {
			mediaGlow(p, glyphs, gx, by, e.glowEm()*size, fill, pose.glow)
		}
		p.Glyphs(glyphs, gx, by, base)
		reach := 0.15 * size
		mediaFill(p, w.boxes, x, y, prog, edge, rtl, fill, func(c ui.Color) { p.Glyphs(glyphs, gx, by, c) },
			func(r ui.Rect) (ui.Rect, bool) {
				left, right := cx+(r.X-c0)*sq, cx+(r.X+r.W-c0)*sq
				if right < gx-reach || left > gx+l.w*sq+reach {
					return ui.Rect{}, false
				}
				return ui.Rect{X: left, Y: by - 2*size, W: right - left, H: 4 * size}, true
			})
	}
}

// glowTaps are where the halo's copies of the glyphs lie, as shares of its
// radius: points spread by a spiral over a disc, at distances that follow a
// Gaussian whose reach is the radius, so that the copies, each faint, add up
// to what a blur of the glyphs' shape would. Copies at low alpha, not a blur
// pass, are what draws it.
var glowTaps = func() (taps [glowTapCount][2]float32) {
	const golden = 2.399963 // radians: the golden angle
	for i := range taps {
		u := (float64(i) + 0.5) / glowTapCount
		rho := min(math.Sqrt(-2*math.Log(1-u*0.97))/2, 1) // sigma is half the radius
		taps[i] = [2]float32{float32(rho * math.Cos(float64(i)*golden)), float32(rho * math.Sin(float64(i)*golden))}
	}
	return taps
}()

// glowTapCount is how many copies make a halo, and glowTapAlpha the share of
// its strength each has.
const (
	glowTapCount = 28
	glowTapAlpha = 0.05
)

// mediaGlow paints a halo of col around glyphs drawn at (x, y): level (0 to
// 1) is its strength and radius how far it reaches, in DIPs.
func mediaGlow(p *ui.Painter, glyphs []ui.Glyph, x, y, radius float32, col ui.Color, level float32) {
	c := mediaAlpha(col, level*glowTapAlpha)
	for _, d := range glowTaps {
		p.Glyphs(glyphs, x+d[0]*radius, y+d[1]*radius, c)
	}
}
