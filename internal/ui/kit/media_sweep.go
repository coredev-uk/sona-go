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

// SweepText is one word-timed text of a lyric line (its lead or its
// background vocals), shaped once for the sheet's width: each word fills
// with the sweep colour from its start to its end as it is sung, short
// words too, behind the soft edge (index.css's .lyric-word), and its letters
// rise with the colour: each starts to lift as the front of the fill reaches
// it and has lifted liftEm (twice that for background vocals) when the front
// has passed it and its soft edge; a held word swells letter by letter with
// a glow on top (media_wordmotion.go). Build one per line and width, keep it
// (shaping is the cost), and paint it from the clock in a Draw callback.
type SweepText struct {
	words  []sweepWord
	layout *ui.TextLayout
	font   ui.Font
	width  float32
	h      float32
	size   float32

	begin  time.Duration // when the first word begins
	empEnd time.Duration // when the last swell has run and settled
}

// sweepWord is one timed word: its boxes in the layout, wrapped words in
// several, its letters and how it moves.
type sweepWord struct {
	text       string
	begin, end time.Duration
	d          time.Duration // how long the colour takes to cross it: end − begin, at least 1 ms
	up         float32       // how far a letter rests raised, in em
	total      float32       // the length of its boxes, in reading order
	boxes      []fillBox
	letters    []sweepLetter
	emp        *sweepSwell // nil for a word that only lifts
}

// sweepSwell is a held word's swell, over its letters.
type sweepSwell struct {
	swell
	run time.Duration // how long the whole swell takes
}

// sweepLetter is one letter (a grapheme) of a word: the unit that rises. A
// word whose letters cannot be told apart in the layout (right to left, whose
// letters join) has one unit per box instead, with no glyphs of its own.
type sweepLetter struct {
	r           ui.Rect // its box in the layout
	x, w        float32
	box         int  // which of the word's boxes it is in
	first, last bool // the first and the last of its box
	baseline    float32
	centre      float32 // y of the middle of its box, which a swelling letter scales about
	// Where it lies along the word in reading order, left to right (l, t:
	// its leading and trailing edge) and right to left (lm, tm).
	l, t, lm, tm float32
	delay        time.Duration // how long after the line stops it starts to settle
	// glyphs are its own, shaped once, at each size it is drawn at (index k:
	// 1 + swellStep·k times the text's size; one only for a letter that does
	// not swell). nil: drawn with its box's text.
	glyphs [][]ui.Glyph
}

// SweepClock is where the voice stands against a SweepText at one frame.
type SweepClock struct {
	// T is the word clock: the song's position under the lyrics' offset.
	T time.Duration
	// Sung (0 to 1) scales the fill's strength, which is how a line fades
	// in and out; 0 paints the text in its base colour alone.
	Sung float32
	// Until is when the line stops being sung (NoUntil: not before the song
	// ends). The letters that lifted settle back from there, and the line is
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
	lines := s.layout.Lines()
	at := 0
	last := -1 // the last word with something to show
	for i, w := range words {
		if strings.TrimSpace(w.Text) != "" {
			last = i
		}
	}
	g := 0 // the letter's place in the text: the order they settle in
	for i, w := range words {
		// The word's boxes, by UTF-16 offsets, without its trailing space.
		units := utf16.Encode([]rune(w.Text))
		trimmed := len(units)
		for trimmed > 0 && (units[trimmed-1] == ' ' || units[trimmed-1] == '\u00a0') {
			trimmed--
		}
		sw := &s.words[i]
		sw.text, sw.begin, sw.end = w.Text, w.Begin, w.End
		sw.d = max(w.End-w.Begin, time.Millisecond)
		sw.up = liftEm
		if w.Background {
			sw.up = 2 * liftEm
		}
		sw.boxes = plainBoxes(s.layout.SelectionRects(ui.TextInputRange{Start: at, End: at + trimmed}))
		for k := range sw.boxes {
			sw.boxes[k].above, sw.boxes[k].below = sw.up*size, sw.up*size
			sw.total += sw.boxes[k].r.W
		}
		swells := emphasizes(w.Text, w.End-w.Begin) && len(sw.boxes) == 1
		if !s.lettersOf(sw, at, swells, lines) {
			s.unitsOf(sw, lines)
			swells = false
		}
		if swells {
			sw.emp = &sweepSwell{swell: newSwell(w.End-w.Begin, len(sw.letters), i == last, w.Background)}
			sw.emp.run = sw.emp.total()
		}
		for k := range sw.letters {
			sw.letters[k].delay = min(time.Duration(g)*settleStagger, settleStaggerMax)
			g++
		}
		at += len(units)
	}
	s.padBoxes()
	for i := range s.words {
		w := &s.words[i]
		if i == 0 || w.begin < s.begin {
			s.begin = w.begin
		}
		if w.emp != nil {
			s.empEnd = max(s.empEnd, w.begin+w.emp.run+maxSettle)
		}
	}
	return s
}

// lettersOf gives word w (which starts at UTF-16 offset at) its letters, one
// per grapheme, with their glyphs, and reports whether it could: not for a
// word of a script read right to left, whose letters join, nor where the
// layout does not give each letter a box of its own. swells says the glyphs
// are wanted at every size of the swell.
func (s *SweepText) lettersOf(w *sweepWord, at int, swells bool, lines []ui.TextLine) bool {
	if hasRTL(w.text) {
		return false
	}
	rest := strings.TrimSpace(w.text)
	at += len(utf16.Encode([]rune(w.text[:len(w.text)-len(strings.TrimLeftFunc(w.text, unicode.IsSpace))]))) // past its leading space
	var letters []sweepLetter
	for rest != "" {
		g := firstGrapheme(rest)
		rest = rest[len(g):]
		n := len(utf16.Encode([]rune(g)))
		rects := s.layout.SelectionRects(ui.TextInputRange{Start: at, End: at + n})
		at += n
		if len(rects) != 1 || rects[0].W <= 0 {
			return false
		}
		r := rects[0]
		box := -1
		for b := range w.boxes {
			if bx := w.boxes[b].r; math.Abs(float64(r.Y-bx.Y)) < 1 && r.X >= bx.X-0.5 && r.X+r.W <= bx.X+bx.W+0.5 {
				box = b
				break
			}
		}
		if box < 0 {
			return false
		}
		l := sweepLetter{r: r, x: r.X, w: r.W, box: box, baseline: baselineAt(lines, r.Y), centre: r.Y + r.H/2}
		steps := 1
		if swells {
			steps = swellSteps + 1
		}
		l.glyphs = make([][]ui.Glyph, steps)
		for k := range l.glyphs {
			f := s.font
			f.Size = s.size * (1 + swellStep*float32(k))
			l.glyphs[k] = ui.Shape(g, f)
		}
		letters = append(letters, l)
	}
	if len(letters) == 0 {
		return false
	}
	w.letters = letters
	w.mark()
	return true
}

// unitsOf gives word w one letter for each of its boxes, drawn with the
// layout's own text.
func (s *SweepText) unitsOf(w *sweepWord, lines []ui.TextLine) {
	w.letters = w.letters[:0]
	for b := range w.boxes {
		r := w.boxes[b].r
		w.letters = append(w.letters, sweepLetter{r: r, x: r.X, w: r.W, box: b, baseline: baselineAt(lines, r.Y), centre: r.Y + r.H/2})
	}
	w.mark()
}

// mark works out where each letter lies along the word and which are the
// first and last of their boxes.
func (w *sweepWord) mark() {
	off := float32(0)
	for b := range w.boxes {
		box := w.boxes[b].r
		for i := range w.letters {
			l := &w.letters[i]
			if l.box != b {
				continue
			}
			l.first = i == 0 || w.letters[i-1].box != b
			l.last = i == len(w.letters)-1 || w.letters[i+1].box != b
			l.l = off + (l.x - box.X)
			l.t = l.l + l.w
			l.lm = off + (box.X + box.W - (l.x + l.w))
			l.tm = l.lm + l.w
		}
		off += box.W
	}
}

// baselineAt is the baseline of the layout's line that starts at y.
func baselineAt(lines []ui.TextLine, y float32) (baseline float32) {
	best := float32(math.MaxFloat32)
	for _, ln := range lines {
		if d := float32(math.Abs(float64(ln.Rect.Y - y))); d < best {
			best, baseline = d, ln.Baseline
		}
	}
	return baseline
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
// and so are not drawn one by one.
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

// rise is how far, 0 to 1, letter l of word w has risen s after the word
// begins: it comes up over riseDur from the moment the colour reaches it
// (reach), whatever the word's length, so a slow word's letters answer the
// voice at once instead of creeping up with its fill. rtl reads the word
// from the right.
func (w *sweepWord) rise(l *sweepLetter, s time.Duration, edge float32, rtl bool) float32 {
	if s <= 0 {
		return 0
	}
	return clamp01(float32(s-w.reach(l, edge, rtl)) / float32(riseDur))
}

// reach is how long after the word begins the colour reaches letter l: the
// front of the fill (its soft edge's far side) is at s/d·(length + edge)
// along the word.
func (w *sweepWord) reach(l *sweepLetter, edge float32, rtl bool) time.Duration {
	lo := l.l
	if rtl {
		lo = l.lm
	}
	return time.Duration(float64(w.d) * float64(lo/(w.total+edge)))
}

// lift is how far letter l of word w is raised at t, in em (up), the line
// sung until until, and whether it is moving (still rising, or settling).
// While the line is sung a letter comes up as the colour reaches it. After
// it a letter settles from where it was: each at its turn from the left,
// over settleDur (a swollen word finishes its swell first).
func (w *sweepWord) lift(l *sweepLetter, t, until time.Duration, edge float32, rtl bool) (lift float32, moving bool) {
	stop := until
	if w.emp != nil {
		stop = max(until, w.begin+w.emp.run)
	}
	if t <= stop {
		s := t - w.begin
		return w.up * riseEase.at(w.rise(l, s, edge, rtl)), s >= 0 && s < w.d+riseDur
	}
	v0 := w.up * riseEase.at(w.rise(l, stop-w.begin, edge, rtl))
	if v0 == 0 {
		return 0, false
	}
	x := float32(t-stop-l.delay) / float32(settleDur)
	if x >= 1 {
		return 0, false
	}
	return v0 * (1 - riseEase.at(clamp01(x))), true
}

// lifts reports whether any letter may be away from its place at c: from the
// first word's begin until the last has settled. Outside it the text is
// painted whole.
func (s *SweepText) lifts(c SweepClock) bool {
	switch {
	case c.Still || c.T < s.begin:
		return false
	case c.T <= c.Until:
		return true
	}
	return c.T-c.Until < maxSettle || c.T < s.empEnd
}

// progress is how far the fill has gone through the word at t, and whether it
// still goes.
func (w *sweepWord) progress(t time.Duration) (float32, bool) {
	if t < w.end && w.end > w.begin {
		return float32(t-w.begin) / float32(w.end-w.begin), true
	}
	return 1, false
}

// Paint draws the text at (x, y) in base, then each word sung by clock.T over
// it in fill, a word being sung up to its soft edge; clock.Sung (0 to 1)
// scales the fill's strength, which is how a line fades in and out. The
// letters rise with the front of the fill, and a held word swells; the fill
// moves with its letters. It reports whether anything is moving, which needs
// the next frame (Painter.AnimationFrame): filling, rising, swelling or
// settling. rtl fills from the right.
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
		prog := float32(0)
		if clock.Sung > 0 && clock.T >= w.begin {
			var sweeping bool
			prog, sweeping = w.progress(clock.T)
			moving = moving || sweeping
		}
		if s.paintWord(p, x, y, w, clock, base, hi, prog, edge, rtl) {
			moving = true
		}
	}
	return moving
}

// paintWord paints word w letter by letter: the letters at rest (not yet
// risen or risen all the way, in runs of neighbours) with the layout's text,
// clipped to the run and shifted, which keeps them as sharp as any text; the
// letters on the move, one by one, from their own glyphs at the fractional
// offset they have got to. It reports whether any is moving.
func (s *SweepText) paintWord(p *ui.Painter, x, y float32, w *sweepWord, c SweepClock, base, hi ui.Color, prog, edge float32, rtl bool) (moving bool) {
	f := c.T - w.begin
	swelling := w.emp != nil && f >= 0 && f < w.emp.run
	if swelling {
		moving = true
	}
	from, runLift := -1, float32(0)
	for i := range w.letters {
		l := &w.letters[i]
		lift, mv := w.lift(l, c.T, c.Until, edge, rtl)
		moving = moving || mv
		if l.glyphs != nil && (swelling || lift != 0 && lift != w.up) {
			if from >= 0 {
				s.paintRun(p, x, y, w, from, i, runLift, base, hi, prog, edge, rtl)
				from = -1
			}
			pose := letterPose{scale: 1}
			if swelling {
				pose = w.emp.pose(i, f)
			}
			s.paintLetter(p, x, y, w, l, lift, pose, base, hi, prog, edge, rtl)
			continue
		}
		if from >= 0 && (lift != runLift || l.box != w.letters[from].box) {
			s.paintRun(p, x, y, w, from, i, runLift, base, hi, prog, edge, rtl)
			from = -1
		}
		if from < 0 {
			from, runLift = i, lift
		}
	}
	if from >= 0 {
		s.paintRun(p, x, y, w, from, len(w.letters), runLift, base, hi, prog, edge, rtl)
	}
	return moving
}

// paintRun paints letters from to to-1 of word w, side by side in one box and
// raised alike, with the layout's text: the base, then the fill where its
// front has reached.
func (s *SweepText) paintRun(p *ui.Painter, x, y float32, w *sweepWord, from, to int, lift float32, base, hi ui.Color, prog, edge float32, rtl bool) {
	a, b := &w.letters[from], &w.letters[to-1]
	box := &w.boxes[a.box]
	x0, x1 := x+a.x, x+b.x+b.w
	if a.first {
		x0 -= box.left
	}
	if b.last {
		x1 += box.right
	}
	run := ui.Rect{X: x0, Y: y + box.r.Y - box.above, W: x1 - x0, H: box.r.H + box.above + box.below}
	dy := -lift * s.size
	draw := func(c ui.Color) { p.TextLayout(s.layout, x, y+dy, c) }
	p.Clip(run, 0, func() { draw(base) })
	if prog <= 0 || hi.A == 0 {
		return
	}
	mediaFill(p, w.boxes, x, y, prog, edge, rtl, hi, draw, func(r ui.Rect) (ui.Rect, bool) {
		lo, hi := max(r.X, run.X), min(r.X+r.W, run.X+run.W)
		top, bot := max(r.Y, run.Y), min(r.Y+r.H, run.Y+run.H)
		return ui.Rect{X: lo, Y: top, W: hi - lo, H: bot - top}, hi > lo && bot > top
	})
}

// paintLetter paints letter l of word w on the move from its own glyphs: raised
// lift em by the fill's front, with the swell's pose on top, at a fractional
// offset (Painter.Transform's translation: smooth, where text positioned
// by Painter.Glyphs lands on whole pixels, at the price of a softer edge
// while it moves). A swelling letter is drawn at its size (the nearest step
// of the swell, shaped once), about its own centre, spread from the word's, with a
// glow of the fill's colour behind it, and the fill passing over it as it moves.
func (s *SweepText) paintLetter(p *ui.Painter, x, y float32, w *sweepWord, l *sweepLetter, lift float32, pose letterPose, base, fill ui.Color, prog, edge float32, rtl bool) {
	size := s.size
	k := min(max(int((pose.scale-1)/swellStep+0.5), 0), len(l.glyphs)-1)
	sq := 1 + swellStep*float32(k)
	c0 := x + l.x + l.w/2 // where the letter's centre is laid out
	cx := c0 + pose.dx*size
	gx := cx - l.w*sq/2
	cy := y + l.centre
	by := cy + (y+l.baseline-cy)*sq // the baseline, scaled about the box's middle
	glyphs := l.glyphs[k]
	reach := 0.15 * size
	p.Transform(ui.Translation(0, (pose.dy-lift)*size), 0, 0, func() {
		if fill.A > 0 && pose.glow > 0.01 {
			mediaGlow(p, glyphs, gx, by, w.emp.glowEm()*size, fill, pose.glow)
		}
		p.Glyphs(glyphs, gx, by, base)
		mediaFill(p, w.boxes, x, y, prog, edge, rtl, fill, func(c ui.Color) { p.Glyphs(glyphs, gx, by, c) },
			func(r ui.Rect) (ui.Rect, bool) {
				left, right := cx+(r.X-c0)*sq, cx+(r.X+r.W-c0)*sq
				if right < gx-reach || left > gx+l.w*sq+reach {
					return ui.Rect{}, false
				}
				return ui.Rect{X: left, Y: by - 2*size, W: right - left, H: 4 * size}, true
			})
	})
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
