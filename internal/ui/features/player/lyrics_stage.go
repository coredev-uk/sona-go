package player

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Full-Screen Lyrics sets the song's phrasing as its layout: lines climb a
// left column, and each line's size comes from how long it is held. The
// numbers below are the contract's (.impeccable/surfaces/…nowplaying-room-go.md).
const (
	// stageSteps is how many sizes a line may have; the held time is
	// quantised to them so the column has rhythm, not noise.
	stageSteps = 5
	// stageMinDIP and stageMaxDIP are the smallest and largest line, at
	// stageRefH of window height; they scale with the window's height.
	stageMinDIP = 40
	stageMaxDIP = 128
	stageRefH   = 1080
	// stagePlainDIP is the one size of lyrics that have no timing.
	stagePlainDIP = 64
	// stageFloorDIP is the smallest a line gets in a small window.
	stageFloorDIP = 22
	// stageMaxWrap is how many lines a line may wrap to before its size steps
	// down.
	stageMaxWrap = 2
	// stageLeft and stageWidth place the column: it starts at 8% of the
	// window's width and is 70% of it wide at most.
	stageLeft  = 0.08
	stageWidth = 0.70
	// stageFocus is where the sung line sits: its middle at 45% of the height.
	stageFocus = 0.45
	// stageSpreadMs is the least spread of held times that makes sizes differ.
	stageSpreadMs = 800
	// stageReach is how many lines before and after the sung one are built:
	// the two shown each way and one more, which is at rest and gone, so that
	// a line fades in and out as the column climbs.
	stageReach = 3
	// stageCut is how far the sung line may move and still be a climb; a seek
	// further than that is a cut.
	stageCut = 3
	// stageSub is the size of a line's background vocals, romanisation and
	// translation, as a share of the line's.
	stageSub = 0.42
	// stageIndent is a duet's second voice's indent, in ems.
	stageIndent = 1.5
	// stageLineH is the line height, in ems.
	stageLineH = 1.05
)

// stageGeom is the stage's size: what the lines are shaped for.
type stageGeom struct {
	w, h, scale float32
	left, colW  float32
	focus       float32
}

func newStageGeom(w, h float32) stageGeom {
	return stageGeom{
		w: w, h: h, scale: h / stageRefH,
		left: round32(stageLeft * w), colW: round32(stageWidth * w), focus: stageFocus * h,
	}
}

// heldSteps is the size step (0 to stageSteps-1) of each line, from how long
// it is held: its span (a word-timed line's from its first word to its last).
// The longest held lines of the song get the top step and the quickest the
// bottom one, found between the 10th and the 90th percentile so one long
// intro doesn't flatten the rest; where those are alike (a song of even
// lines with one held one) the top is the longest line. A song whose lines
// are all held alike sets every line at the middle step.
func heldSteps(spans []lyricsync.Span) []int {
	steps := make([]int, len(spans))
	mid := stageSteps / 2
	var held []int64
	for _, s := range spans {
		if d := s.EndMs - s.BeginMs; d > 0 {
			held = append(held, d)
		}
	}
	slices.Sort(held)
	var lo, hi, median int64
	if len(held) > 0 {
		lo, hi, median = held[(len(held)-1)/10], held[(len(held)-1)*9/10], held[len(held)/2]
		if hi-lo < stageSpreadMs {
			hi = held[len(held)-1]
		}
	}
	for i, s := range spans {
		d := s.EndMs - s.BeginMs
		if d <= 0 {
			d = median
		}
		if hi-lo < stageSpreadMs {
			steps[i] = mid
			continue
		}
		t := float64(min(max(d, lo), hi)-lo) / float64(hi-lo)
		steps[i] = int(math.Round(t * (stageSteps - 1)))
	}
	return steps
}

// stageSize is the type size of a step at a window's scale (its height over
// stageRefH).
func stageSize(step int, scale float32) float32 {
	dip := float32(stageMinDIP) + float32(step)*(stageMaxDIP-stageMinDIP)/(stageSteps-1)
	return max(round32(dip*scale), stageFloorDIP)
}

// stageWeight: the large steps are set heavier.
func stageWeight(step int) int {
	if step >= stageSteps-2 {
		return 800
	}
	return 700
}

// stageKey names one shaped text of a line.
type stageKey struct {
	line          int
	part          uint8 // 0 lead, 1 background, 2 pronunciation, 3 translation
	italic, whole bool
	size, width   float32
}

// stageBlock is one line shaped for the stage: its texts, its size and its
// height.
type stageBlock struct {
	step           int // -1 for plain lyrics
	size, lineH    float32
	indent         float32
	lead           *kit.SweepText
	bg, pron, tr   *kit.SweepText
	shown          lyricsync.ShownLine
	leadTimed      bool
	other, headed  bool    // the duet's second voice; "Together" stands above it
	subGap         float32 // between the lead and each text under it
	h              float32 // the texts, without the heading
	headH, headGap float32 // the "Together" heading's height and its gap to the line
}

// lines is how many lines the lead wrapped to.
func (b *stageBlock) lines() int { return int(math.Round(float64(b.lead.Height() / b.lineH))) }

// stage is Full-Screen Lyrics of one song: the lyrics' derived data, the
// lines shaped for the stage's size, where each of them stands in the column
// and the chrome's idle clock. A new song gets a new one. Main thread only.
type stage struct {
	songID string
	lyricsModel
	steps []int
	duet  bool

	// fitted says what the shaped lines are for: they are shaped again
	// when it changes.
	fitted  bool
	g       stageGeom
	prefs   lyricsPrefsView
	reduced bool
	cache   map[stageKey]*kit.SweepText
	blocks  map[int]*stageBlock

	// The column: line origin+k's body starts tops[k] down from the origin.
	// It is laid out as far as the sung line needs, so a long song is not
	// shaped when it opens in the middle.
	origin int
	tops   []float32

	// The last frame's picture, for the keys and the tests.
	anchor, active int
	cursor         int // plain lyrics: the line the reader is at
	cut            bool

	// The wash's drift: started, and when it began.
	driftAt time.Time
	drifted bool

	// The chrome: the last time the pointer moved or a key was pressed (zero:
	// not yet, so it starts hidden), where the pointer was (ptrSeen: it has
	// been over the view), and whether the strip held the focus when it was
	// built last.
	wake         time.Time
	ptrX, ptrY   float32
	ptrSeen      bool
	stripFocused bool
}

func newStage(songID string) *stage {
	return &stage{songID: songID, anchor: -1, active: -1}
}

// load derives what the stage needs from the lyrics, once per lyrics.
func (s *stage) load(l *services.Lyrics) {
	if !s.bind(l) {
		return
	}
	s.steps = nil
	if s.synced {
		s.steps = heldSteps(s.spans)
	}
	s.duet = false
	for _, v := range s.voices {
		s.duet = s.duet || v == lyricsync.VoiceTwo
	}
	s.fitted = false
}

// carry takes over the chrome's state from the stage of the song before, so
// that a song change does not bring the strip back or start the drift again.
func (s *stage) carry(from *stage) {
	s.wake, s.ptrX, s.ptrY, s.ptrSeen = from.wake, from.ptrX, from.ptrY, from.ptrSeen
	s.driftAt, s.drifted, s.stripFocused = from.driftAt, from.drifted, from.stripFocused
}

// fit shapes for g, the choices and the motion setting: anything shaped for
// others is dropped.
func (s *stage) fit(g stageGeom, prefs lyricsPrefsView, reduced bool) {
	if s.fitted && s.g == g && s.prefs == prefs && s.reduced == reduced {
		return
	}
	s.fitted, s.g, s.prefs, s.reduced = true, g, prefs, reduced
	s.cache = map[stageKey]*kit.SweepText{}
	s.blocks = map[int]*stageBlock{}
	s.tops = s.tops[:0]
}

// sweep is the shaped text of a part of line i, shaped once for its size and
// width.
func (s *stage) sweep(k stageKey, ws []lyrics.Word, weight int, lineH float32) *kit.SweepText {
	if sw := s.cache[k]; sw != nil {
		return sw
	}
	lang := s.partLang(k.part)
	var sw *kit.SweepText
	if k.italic {
		sw = kit.NewSweepTextItalic(ws, lang, k.size, weight, lineH, k.width)
	} else {
		sw = kit.NewSweepText(ws, lang, k.size, weight, lineH, k.width)
	}
	s.cache[k] = sw
	return sw
}

// words are a text's words as a sweep takes them: the timed words, or the
// text as one word that is never lit.
func (s *stage) words(t lyricsync.Text) ([]lyrics.Word, bool) {
	if s.wordTimed && len(t.Words) > 0 {
		return sweepWords(t.Words, s.reduced), true
	}
	return []lyrics.Word{{Text: t.Text}}, false
}

// block is line i shaped: at its size step, or the next one down while it
// would wrap to more than stageMaxWrap lines.
func (s *stage) block(i int) *stageBlock {
	if b := s.blocks[i]; b != nil {
		return b
	}
	step := -1
	if s.steps != nil {
		step = s.steps[i]
	}
	b := s.shape(i, step)
	for step > 0 && b.lines() > stageMaxWrap {
		step--
		b = s.shape(i, step)
	}
	s.blocks[i] = b
	return b
}

func (s *stage) shape(i, step int) *stageBlock {
	ln := &s.lyr.Lines[i]
	g := s.g
	size, weight := stageSize(max(step, 0), g.scale), stageWeight(step)
	if step < 0 {
		size, weight = max(round32(stagePlainDIP*g.scale), stageFloorDIP), 700
	}
	b := &stageBlock{step: step, size: size, lineH: round32(size * stageLineH)}
	b.shown = lyricsync.Shown(ln, s.prefs)
	b.other = s.voices[i] == lyricsync.VoiceTwo
	b.headed = s.duet && s.voices[i] == lyricsync.VoiceAll && (i == 0 || s.voices[i-1] != lyricsync.VoiceAll)
	if b.other {
		b.indent = round32(size * stageIndent)
	}
	width := float32(math.Floor(float64((g.colW-b.indent)/sweepStep))) * sweepStep
	width = max(width, sweepStep)

	var ws []lyrics.Word
	ws, b.leadTimed = s.words(b.shown.Lead)
	b.lead = s.sweep(stageKey{i, 0, b.other, s.reduced, size, width}, ws, weight, b.lineH)
	b.h = b.lead.Height()

	sub := max(round32(size*stageSub), 15)
	subH := round32(sub * 1.25)
	b.subGap = round32(size * 0.12)
	part := func(k uint8, t lyricsync.Text, weight int) *kit.SweepText {
		ws, _ := s.words(t)
		sw := s.sweep(stageKey{i, k, b.other, s.reduced, sub, width}, ws, weight, subH)
		b.h += b.subGap + sw.Height()
		return sw
	}
	if t := b.shown.Background; t.Text != "" || len(t.Words) > 0 {
		b.bg = part(1, t, 600)
	}
	if t := b.shown.Pronunciation; t != nil {
		b.pron = part(2, *t, 400)
	}
	if t := b.shown.Translation; t != "" {
		b.tr = part(3, lyricsync.Text{Text: t}, 400)
	}
	if b.headed {
		hs := max(round32(18*g.scale), 14)
		b.headH, b.headGap = round32(hs*1.3), 6
	}
	return b
}

// gap is the space between line i-1's body and line i's: a verse break, by
// the sheet's rule (a pause of three seconds, or a change of song part: 0.9
// em), or a line's.
func (s *stage) gap(i int) float32 {
	a, b := s.block(i-1), s.block(i)
	em := (a.size + b.size) / 2
	if s.verseOf[i] != s.verseOf[i-1] {
		return 0.9 * min(em, 72*s.g.scale)
	}
	return round32(0.14 * em)
}

// reach lays the column out as far as lines lo to hi. It reports whether it
// started again (the lines' places are not those of the frame before).
func (s *stage) reach(lo, hi int) (restarted bool) {
	if len(s.tops) == 0 || lo < s.origin || lo > s.origin+len(s.tops) {
		s.origin, s.tops, restarted = lo, s.tops[:0], true
	}
	for s.origin+len(s.tops) <= hi {
		i := s.origin + len(s.tops)
		top := float32(0)
		if len(s.tops) > 0 {
			prev := s.block(i - 1)
			top = s.tops[len(s.tops)-1] + prev.h + s.gap(i) + s.block(i).headH + s.block(i).headGap
		} else {
			top = s.block(i).headH + s.block(i).headGap
		}
		s.tops = append(s.tops, top)
	}
	return restarted
}

// top is where line i's body stands in the column (reach has laid it out).
func (s *stage) top(i int) float32 { return s.tops[i-s.origin] }

// climb is how the column goes to a new place when the sung line advances:
// the climb token (600 ms, exponential out).
func climb() ui.Animation {
	m := theme.Current().Motion["climb"]
	return ui.Animation{Duration: m.Duration, Ease: m.Ease}
}

// stagePose is where a line stands by its rank (how many lines it is from
// the one in the middle): the sung line at full size, the others a little
// smaller and fainter by the distance, the lines beyond the two shown each
// way gone. The ink says which are past and which are coming; this only
// thins the far ones.
func stagePose(rank int) ui.Pose {
	r := min(abs(rank), stageReach)
	var fade float32
	switch {
	case r >= stageReach:
		fade = 1
	case rank == -2:
		fade = 0.45
	case rank == 2:
		fade = 0.3
	}
	return ui.Pose{Scale: 1 - 0.04*float32(r), Fade: fade}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// stageIn is what one build of the cascade needs.
type stageIn struct {
	g       stageGeom
	anchor  int // the line in the middle
	active  int // the line sung (-1: none, as in a break)
	reduced bool
}

// cascade builds the column: the sung line in the middle and the lines
// around it, each at its own size. The lines stand where they stand in the
// column; the column as a whole is posed so the sung line is in the middle,
// and a new pose climbs there over 600 ms in frames that only paint. The
// view is built once per line, not once per frame.
func (p *player) cascade(c *ui.Context, st *stage, in stageIn) {
	n := len(st.lyr.Lines)
	lo, hi := max(in.anchor-stageReach, 0), min(in.anchor+stageReach, n-1)
	restarted := st.reach(lo, hi)
	cut := restarted || st.anchor < 0 || abs(in.anchor-st.anchor) > stageCut
	st.anchor, st.active, st.cut = in.anchor, in.active, cut
	if in.reduced || cut {
		// Lines jump into place: under reduced motion always, and after a
		// seek that moves the column further than it climbs.
		c.DisableAnimations()
	}

	// The middle of the sung line itself: what stands under it (background
	// vocals, the translation) does not move it.
	mid := st.top(in.anchor) + st.block(in.anchor).lead.Height()/2
	box := ui.Box(c.Key("cascade:" + st.songID)).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(true).
		Role(ui.RoleGroup).Label(i18n.T("Lyrics"))
	if !in.reduced {
		m := theme.Current().Motion["backdrop"]
		box.Transition(ui.ElementTransition{
			Colors: true, Enter: &ui.Motion{Y: 48 * in.g.scale}, Exit: &ui.Motion{}, Duration: m.Duration, Ease: m.Ease,
		})
	}
	box.AnimatePose(ui.Pose{Y: in.g.focus - mid}, climb())
	box.Children(func() {
		for i := lo; i <= hi; i++ {
			p.stageLine(c, st, in, i)
		}
	})
}

// lineState is where line i stands against the voice.
func (st *stage) lineState(i int, in stageIn) lineState {
	switch {
	case !st.synced:
		return stPlain
	case i == in.active:
		return stActive
	case i < max(in.active, in.anchor):
		return stPast
	}
	return stFuture
}

// stageLine is one line of the column: the main vocal, then the background
// vocals, the romanisation and the translation under it, in the colours of
// the sheet (the sung line in ink and its words in amber as the voice
// reaches them; earlier lines faint, later ones muted). A duet's second voice
// is indented 1.5em and italic, and the first line sung together has its
// "Together" heading above it.
func (p *player) stageLine(c *ui.Context, st *stage, in stageIn, i int) {
	b := st.block(i)
	state := st.lineState(i, in)
	tone := lineTone(state, len(st.lyr.Lines[i].Words) > 0)
	sub := theme.C("ink-muted")
	if state == stPast {
		sub = theme.C("ink-faint")
	}
	fill := theme.C("amber-ink")
	g := in.g

	col := ui.Column(c.Key("line:" + strconv.Itoa(i))).Absolute().Left(g.left + b.indent).Top(st.top(i) - b.headH - b.headGap).
		Width(g.colW - b.indent).Height(b.headH + b.headGap + b.h).IgnoreInput(true)
	col.TransformOrigin(0, 0.5)
	col.AnimatePose(stagePose(i-in.anchor), climb())
	col.Children(func() {
		if b.headed {
			lineText(c.Key("together"), i18n.T("Together"), b.headH/1.3, b.headH, 500, theme.C("ink-muted")).Margin(0, 0, b.headGap, 0)
		}
		part := func(key string, sw *kit.SweepText, t lyricsync.Text, k uint8, base ui.Color, margin float32) {
			e := ui.Box(c.Key(key)).FillWidth().Height(sw.Height()).Margin(margin, 0, 0, 0).Role(ui.RoleText).Label(t.Text).Lang(st.partLang(k))
			if st.wordTimed && len(t.Words) > 0 {
				e.Draw(p.sweepDraw(sw, st.songID, st.spans, i, t, base, fill, in.reduced))
				return
			}
			right := rtl(t.Text)
			e.Draw(func(pt *ui.Painter, r ui.Rect) {
				x := r.X
				if right {
					x = r.X + r.W - sw.Width()
				}
				sw.Paint(pt, x, r.Y, base, base, 0, 0, right)
			})
		}
		part("lead", b.lead, b.shown.Lead, 0, tone, 0)
		if b.bg != nil {
			part("bg", b.bg, b.shown.Background, 1, sub, b.subGap)
		}
		if b.pron != nil {
			part("pron", b.pron, *b.shown.Pronunciation, 2, sub, b.subGap)
		}
		if b.tr != nil {
			part("tr", b.tr, lyricsync.Text{Text: b.shown.Translation}, 3, sub, b.subGap)
		}
	})
}
