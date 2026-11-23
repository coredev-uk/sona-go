package player

import (
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The reading position, as a fraction of the scroller's height: the sung
// line stays there, a third of the way down, and the lines glide past it one
// by one as the voice goes on (follow).
const (
	readAt = 1.0 / 3
	// cutFar is how far, in sheet heights, a change of line may move the
	// sheet and still glide; a seek further than that cuts.
	cutFar = 1.0
	// cutLines is how many lines a change of line may move the sung one and
	// still glide; a seek or a click further than that cuts, so that a long
	// glide never sweeps the whole sheet.
	cutLines = 6
	// rebaseWait is how long a rebase waits for the user's own scroll to come
	// to rest.
	rebaseWait = 120 * time.Millisecond
	// rebaseSlop is how early a build may take the lines for at rest: the
	// window's frames and the player's clock do not tick together, and the
	// margin of cascadeSettle leaves the glide long finished.
	rebaseSlop = 30 * time.Millisecond
	// userScroll is how long the sheet is left alone after the user scrolled it.
	userScroll = 4 * time.Second
	// sweepStep: a line's shaped text is kept per width rounded down to this,
	// so a window drag reshapes rarely.
	sweepStep = 8
)

// lineState is where a line stands against the voice; stPlain is for lyrics
// without timing.
type lineState int

const (
	stPast lineState = iota
	stActive
	stFuture
	stPlain
)

// lineTone is a line's text colour (useLyricsView.ts lineTone): the sung line
// in ink (its words fill with amber) or, timed by line only, amber itself;
// earlier lines faint, later ones muted, plain lyrics in ink. No transition
// between states.
func lineTone(state lineState, wordTimed bool) ui.Color {
	switch {
	case state == stActive && wordTimed:
		return theme.C("ink")
	case state == stActive:
		return theme.C("play-ink")
	case state == stPast:
		return theme.C("ink-faint")
	case state == stFuture:
		return theme.C("ink-muted")
	}
	return theme.C("ink")
}

// lyricLook is the type and spacing of a sheet (Now Playing's).
type lyricLook struct {
	size, lineH       float32 // the main vocal
	weight            int
	subSize, subLineH float32 // background vocals, pronunciation, translation
	padX, padY        float32
	subGap, verseGap  float32
	indent            float32 // extra start padding of a duet's other voice
	togetherSize      float32
	topPad, botPad    float32 // the sheet's padding, as fractions of its height
	fadeDIP           float32 // the height of the fade at the top and bottom
}

// sheetLook is the Now Playing sheet: 24/30 type, lines padded 6×12.
func sheetLook() lyricLook {
	return lyricLook{
		size: 24, lineH: 30, weight: 650, subSize: 16, subLineH: 22, padX: 12, padY: 6,
		subGap: 2, verseGap: 32, indent: 36, togetherSize: 12, topPad: readAt, botPad: 0.70, fadeDIP: 48,
	}
}

// sweepKey names one shaped text of a line.
type sweepKey struct {
	line   int
	part   uint8 // 0 lead, 1 background, 2 pronunciation
	italic bool
	whole  bool
}

// lyricsSheet is one lyric sheet's state: the scroller, the cascade's
// bookkeeping, the keyboard focus and the shaped sweeps. A new song gets a
// new one (v0.2.0 remounted the pane per song). Main thread only.
type lyricsSheet struct {
	songID string

	scroll  ui.ScrollState
	box     ui.Handle
	content ui.Handle // the column inside the scroller, whose top is where the lines' places are counted from
	lines   []ui.Handle
	gapAt   []ui.Handle // the dots of an instrumental gap before a line

	lastY      float32
	wasGliding bool
	userAt     time.Time
	anchored   int
	jumped     bool

	// The cascade (follow). The scroll is put where the sung line stands at
	// the reading position as the line changes, and every line is posed back
	// to where it was and glides on from there: off is how far the lines are
	// posed from where the layout has them (the scroll is that far short of
	// where the sung line wants it), up the way they go, settleAt when they
	// are all at rest, and poseLo..poseHi the lines posed (those on screen,
	// before or after, and the ones about to be).
	off            float32
	up             bool
	settleAt       time.Time
	poseLo, poseHi int
	pad            float32 // the sheet's top padding as the last build set it
	seen           int     // the anchor of the last build
	tries          int     // the frames asked for before the first layout let the sheet take its place

	// focused is the line that had the focus last (the roving tab stop), -1
	// before any; want is the line a key asked the focus for.
	focused, want int

	lyricsModel
	sweeps map[sweepKey]*kit.SweepText

	// Last frame's picture, for tests and for the cascade.
	active, anchor int
	width          float32
}

func newLyricsSheet(songID string) *lyricsSheet {
	return &lyricsSheet{songID: songID, anchored: -1, seen: -1, focused: -1, want: -1}
}

// lyricsModel is what a view of lyrics derives from them, once per lyrics:
// the sheet in Now Playing and full-screen lyrics share it.
type lyricsModel struct {
	lyr       *services.Lyrics
	spans     []lyricsync.Span
	voices    []lyricsync.Voice
	verseOf   []int
	verseList [][]int
	gaps      []lyricsync.Span // the instrumental gap before each line (the zero Span: none); synced lyrics only
	synced    bool
	wordTimed bool
}

// bind derives what a view needs from the lyrics, once per lyrics. The query
// hands out a copy of the DTO each frame, so the lines' backing array says
// whether they are the same lyrics. It reports whether these are new lyrics.
func (m *lyricsModel) bind(l *services.Lyrics) bool {
	if m.lyr != nil && lyricsync.SameLines(m.lyr.Lines, l.Lines) {
		m.lyr = l
		return false
	}
	m.lyr = l
	m.spans = lyricsync.LineSpans(l.Lines)
	m.voices = lyricsync.LineVoices(l.Lines)
	m.synced = lyricsync.IsSynced(l)
	m.wordTimed = lyricsync.HasWordTiming(l)
	m.verseList = lyricsync.Verses(l.Lines, m.spans, m.synced)
	m.gaps = nil
	if m.synced {
		m.gaps = lyricsync.Gaps(m.spans)
	}
	m.verseOf = make([]int, len(l.Lines))
	for v, ids := range m.verseList {
		for _, i := range ids {
			m.verseOf[i] = v
		}
	}
	return true
}

// bind derives the sheet's own state from the lyrics, once per lyrics.
func (sh *lyricsSheet) bind(l *services.Lyrics) {
	if !sh.lyricsModel.bind(l) {
		return
	}
	sh.lines = make([]ui.Handle, len(l.Lines))
	sh.gapAt = make([]ui.Handle, len(l.Lines))
	sh.sweeps = map[sweepKey]*kit.SweepText{}
}

// partLang is the language of one part of a line: the lyrics' own for the
// lead and background vocals, the romanisation's for the pronunciation and
// the translation's for the translation (part 3, which only full-screen
// lyrics shape as a part). "" leaves it to the app's.
func (m *lyricsModel) partLang(part uint8) string {
	switch part {
	case 2:
		return m.lyr.TransliterationLanguage
	case 3:
		return m.lyr.TranslationLanguage
	}
	return m.lyr.Language
}

// sweep is the shaped text of one part of line i for width, shaped again
// only for a new width.
func (sh *lyricsSheet) sweep(k sweepKey, words []services.LyricWord, look lyricLook, bg bool, w float32) *kit.SweepText {
	if s := sh.sweeps[k]; s != nil && s.Width() == w {
		return s
	}
	size, lineH, weight := look.size, look.lineH, look.weight
	if k.part != 0 {
		size, lineH, weight = look.subSize, look.subLineH, 400
		if bg {
			weight = 600
		}
	}
	ws := sweepWords(words, k.whole)
	lang := sh.partLang(k.part)
	var s *kit.SweepText
	if k.italic {
		s = kit.NewSweepTextItalic(ws, lang, size, weight, lineH, w)
	} else {
		s = kit.NewSweepText(ws, lang, size, weight, lineH, w)
	}
	sh.sweeps[k] = s
	return s
}

// lineText is text in an explicit size: the sheets' lines are not in the
// theme's type scale.
func lineText(c *ui.Context, s string, size, lineH float32, weight int, col ui.Color) ui.Element {
	return ui.Text(c, s).FontSize(size).FixedLineHeight(lineH).FontWeight(weight).TextColor(col)
}

// sheetIn is what one build of a sheet needs from its caller.
type sheetIn struct {
	look lyricLook
	// clickable makes synced lines buttons that seek (Now Playing's).
	clickable bool
	// wash is the colour of what the sheet sits on at a point of the window
	// (the cover's wash), which its fades go to; nil: the ground.
	wash func(x, y float32) ui.Color
}

// washOf is the cover's wash behind Now Playing and Full-Screen Lyrics.
func washOf(cur *services.QueueItem) kit.BackdropOpts {
	return kit.BackdropOpts{Src: cur.Backdrop, Tone: kit.ToneGround}
}

// washAt is the colour of the wash at a point of the window, for the view
// whose root is root: what the sheet's fades go to. It reads the wash now,
// so a painter may call it later; nil when there is no song.
func (p *player) washAt(c *ui.Context, root ui.Handle) func(x, y float32) ui.Color {
	cur := p.env.Play.State().Current
	if cur == nil {
		return nil
	}
	w, box := p.env.Art.WashOf(washOf(cur)), root.Bounds(c)
	return func(x, y float32) ui.Color { return w.At(box, x, y) }
}

// fadeStep is the width of a piece of a fade, in DIPs: the wash's picture is
// 64 px across, so it changes little within this.
const fadeStep = 16

// paintFade paints a sheet's fade into r: opaque at the sheet's edge (the
// top of r when top, else the bottom), clear at the other, in the colour of
// the wash there (the ground without one).
func paintFade(p *ui.Painter, r ui.Rect, wash func(x, y float32) ui.Color, top bool) {
	edge := r.Y + r.H
	if top {
		edge = r.Y
	}
	n := 1
	if wash != nil {
		n = min(max(int(math.Ceil(float64(r.W/fadeStep))), 1), 128)
	}
	for i := range n {
		x0, x1 := r.X+round32(r.W*float32(i)/float32(n)), r.X+round32(r.W*float32(i+1)/float32(n))
		solid := theme.C("ground")
		if wash != nil {
			solid = wash((x0+x1)/2, edge)
		}
		from, to := solid, solid.Alpha(0)
		if !top {
			from, to = to, from
		}
		p.FillGradient(ui.Rect{X: x0, Y: r.Y, W: x1 - x0, H: r.H}, ui.LinearGradient{From: from, To: to, Angle: 180}, 0)
	}
}

func round32(v float32) float32 { return float32(math.Round(float64(v))) }

// lyricsScroller builds the sheet: the lines in verses inside a scroller whose
// lines glide, one by one, to keep the sung line at the reading position,
// and which fades out at its top and bottom. It returns the scroller.
func (p *player) lyricsScroller(c *ui.Context, sh *lyricsSheet, l *services.Lyrics, in sheetIn) ui.Element {
	sh.bind(l)
	look := in.look
	prefs := p.prefsFor().get()
	reduced := theme.ReducedMotion()
	now := p.now()
	s := p.env.Play.State()
	playing := s.State == "playing"
	posMs := p.env.Play.Position().Milliseconds()
	off := lyricsync.Session().Get(sh.songID)

	sung := lyricsync.Sheet{Active: -1, Anchor: -1, Next: lyricsync.Never}
	clock := lyricsync.LyricsClock(posMs, off)
	if sh.synced {
		sung = lyricsync.SungLine(sh.spans, clock)
	}
	sh.active, sh.anchor = sung.Active, sung.Anchor
	// A timer waits for the next line or rest instead of polling the position.
	if playing && sung.Next != lyricsync.Never {
		c.After(lyricsync.Dur(max(sung.Next-clock, 0) + 1))
	}
	if playing && reduced && sh.wordTimed {
		p.wakeAtWordChange(c, &sh.lyricsModel, lyricsync.WordClock(posMs, off))
	}

	// The user moved the sheet (wheel, touchpad, scroll bar): leave it alone a while.
	gliding := sh.scroll.Scrolling()
	if !gliding && !sh.wasGliding && sh.scroll.Y != sh.lastY {
		sh.userAt = now
	}
	sh.wasGliding = gliding

	sheet := ui.Scroll(c.Key("lyrics:" + sh.songID)).TrackScroll(&sh.scroll).Grow(1).Basis(0).MinHeight(0).Scrollbars(false).
		Role(ui.RoleGroup).Label(i18n.T("Lyrics"))
	sheet.Bind(&sh.box)
	if !sh.synced {
		// Plain lyrics hold no buttons: the sheet itself takes focus to scroll.
		sheet.Focusable()
	}
	view := sheet.Bounds()
	sh.width = view.W
	text := view.W - 2*look.padX
	dots := sh.synced && sung.Anchor >= 0 && lyricsync.HasGap(sh.gaps[sung.Anchor]) && gapStateOf(sung.Anchor, sung.Active, sung.Anchor) != gapPast
	pad := view.H * look.topPad
	sh.follow(c, view, sung.Anchor, pad, dots, reduced, now)
	sheet.Children(func() {
		ui.Column(c.Key("sheet")).FillWidth().Padding(pad, 0, view.H*look.botPad).Bind(&sh.content).Children(func() {
			ui.Column(c.Key("column")).FillWidth().Children(func() {
				p.lyricVerses(c, sh, in, prefs, sung, text, reduced)
				p.lyricsFooter(c, sh)
			})
		})
	})

	sh.lastY = sh.scroll.Y

	// Fade the sheet out at its top and bottom (.lyrics-fade): no blur. The
	// fades go to the colour of what is behind the sheet, the cover's wash
	// when it has one: cut into narrow pieces across, as the wash changes
	// along the edge.
	if view.H > 0 {
		fade := min(look.fadeDIP, view.H/3)
		wash := in.wash
		ui.Box(c.Key("fade:top")).Absolute().Top(0).Left(0).Right(0).Height(fade).IgnoreInput(true).Draw(func(p *ui.Painter, r ui.Rect) {
			paintFade(p, r, wash, true)
		})
		ui.Box(c.Key("fade:bottom")).Absolute().Bottom(0).Left(0).Right(0).Height(fade).IgnoreInput(true).Draw(func(p *ui.Painter, r ui.Rect) {
			paintFade(p, r, wash, false)
		})
	}
	return sheet
}

// wakeAtWordChange asks for a frame when the next word begins or ends: under
// reduced motion words light whole, and nothing animates in between.
func (p *player) wakeAtWordChange(c *ui.Context, m *lyricsModel, t int64) {
	singing := lyricsync.SingingLines(m.spans, t)
	var words []lyricsync.Span
	for _, i := range singing {
		for _, w := range m.lyr.Lines[i].Words {
			words = append(words, lyricsync.Span{BeginMs: w.BeginMs, EndMs: w.EndMs})
		}
	}
	if next := lyricsync.NextChange(m.spans, singing, words, t); next != lyricsync.Never {
		c.After(lyricsync.Dur(next - t + 1))
	}
}

// follow keeps the sung line (the anchor) at the reading position, line by
// line: when the line changes the scroll goes to where the line stands a third
// of the way down, and every line on screen glides to its new place on its own
// (cascadeAnim), the sung line first and the lines trailing it after it, down
// on a seek back. The glide is the lines' pose, which only paints; the scroll
// itself does not glide.
//
// A pose can be set to a place but not read back, so the scroll goes to the
// new place only once the lines have settled (rebase), in a build of its
// own that jumps: until then the scroll stays where it was and the lines are
// posed off from the layout. The jump of rebase moves the scroll by what
// the pose held, and the pose to rest, so nothing shows. The first line, a
// seek of more than cutFar sheet heights and reduced motion jump to place at
// once. The user's scroll holds it all still (userScroll); a build that finds
// the lines on their way and the line changed again moves on from where they
// are.
func (sh *lyricsSheet) follow(c *ui.Context, view ui.Rect, anchor int, pad float32, dots, reduced bool, now time.Time) {
	sh.poseLo, sh.poseHi = 0, -1
	// The bounds are those of the layout of the build before, whose padding
	// was sh.pad: lines stand pad - sh.pad further down in this one. The
	// first layout has no height to take a third of, and a window being resized
	// changes it: the line is found again.
	shift := pad - sh.pad
	sh.pad = pad
	seen := sh.seen // the line the last build had, which a seek is measured from
	sh.seen = anchor
	box, content := sh.box.Bounds(c), sh.content.Bounds(c)
	if !sh.synced || view.H <= 0 || content.H <= 0 {
		// Nothing to follow, or not laid out yet: the sheet takes its first
		// place in a build of the frame after the layout, not at the next line.
		if sh.synced && !sh.jumped && sh.tries < 3 {
			sh.tries++
			c.Invalidate()
		}
		return
	}
	resized := shift != 0
	if anchor >= 0 && anchor < len(sh.lines) && (sh.anchored != anchor || resized) && now.Sub(sh.userAt) >= userScroll {
		if lb := sh.lines[anchor].Bounds(c); lb.H > 0 {
			top := lb.Y - content.Y + shift
			if db := sh.gapAt[anchor].Bounds(c); dots && db.H > 0 {
				top = db.Y - content.Y + shift // the dots lead the line, and are what is sung first
			}
			want := max(0, top-view.H*readAt)
			if sh.scroll.MaxY > 0 {
				want = min(want, sh.scroll.MaxY+shift)
			}
			sh.anchored = anchor
			from := sh.scroll.Y
			switch {
			case !sh.jumped || resized || reduced || abs(anchor-seen) > cutLines || max(want-from, from-want) > view.H*cutFar:
				c.DisableAnimations()
				sh.scroll.Y, sh.off, sh.settleAt = want, 0, time.Time{}
			case from-want != sh.off:
				sh.up = from-want < sh.off
				sh.off = from - want
				sh.settleAt = now.Add(cascadeSettle())
			}
			sh.jumped = true
		}
	}

	// Rebase: the lines are at rest where the pose has them, so the scroll
	// goes there and the pose to nothing, in one frame that shows no change.
	if sh.off != 0 {
		switch {
		case sh.settleAt.Sub(now) > rebaseSlop:
			c.After(sh.settleAt.Sub(now) + time.Millisecond)
		case sh.scroll.Scrolling():
			c.After(rebaseWait) // the user's glide goes on first
		default:
			c.DisableAnimations()
			sh.scroll.Y -= sh.off
			sh.off = 0
		}
	}

	// The lines to pose: those that are, or were, or will be on screen, with
	// half a sheet to spare, so that a line coming in glides in from where it
	// was and does not appear.
	scrolled := box.Y - content.Y // the scroll the layout used
	lo := min(scrolled, sh.scroll.Y, sh.scroll.Y-sh.off) - view.H/2
	hi := max(scrolled, sh.scroll.Y, sh.scroll.Y-sh.off) + view.H*1.5
	first := true
	for i, h := range sh.lines {
		lb := h.Bounds(c)
		if lb.H <= 0 || lb.Y-content.Y+shift+lb.H < lo || lb.Y-content.Y+shift > hi {
			continue
		}
		if first {
			sh.poseLo, first = i, false
		}
		sh.poseHi = i
	}
}

// posed reports whether line i is posed by the cascade.
func (sh *lyricsSheet) posed(i int) bool { return i >= sh.poseLo && i <= sh.poseHi }

// glide poses element e, line i's (or its dots'), at the shift of the
// cascade, as the line goes after the sung one.
func (sh *lyricsSheet) glide(e ui.Element, i int, reduced bool) {
	if sh.posed(i) {
		e.AnimatePose(ui.Pose{Y: sh.off}, cascadeAnim(i-sh.anchor, sh.up, reduced))
	}
}

// userScrolled records that the user moved the sheet (keys).
func (sh *lyricsSheet) userScrolled(now time.Time) { sh.userAt = now }

// lyricVerses builds the lines, a verse at a time.
func (p *player) lyricVerses(c *ui.Context, sh *lyricsSheet, in sheetIn, prefs lyricsPrefsView, sung lyricsync.Sheet, textW float32, reduced bool) {
	l := sh.lyr
	duet := false
	for _, v := range sh.voices {
		duet = duet || v == lyricsync.VoiceTwo
	}
	tabStop := sh.focused
	if tabStop < 0 {
		tabStop = max(sung.Anchor, 0)
	}
	for v, ids := range sh.verseList {
		verse := ui.Column(c.Key("verse:" + strconv.Itoa(v))).FillWidth()
		if v > 0 {
			verse.Margin(in.look.verseGap, 0, 0, 0)
		}
		verse.Children(func() {
			for _, i := range ids {
				state := stPlain
				if sh.synced {
					switch {
					case i == sung.Active:
						state = stActive
					case i < max(sung.Active, sung.Anchor):
						state = stPast
					default:
						state = stFuture
					}
				}
				if sh.synced && lyricsync.HasGap(sh.gaps[i]) {
					p.lyricGap(c, sh, i, in, gapStateOf(i, sung.Active, sung.Anchor), reduced)
				}
				together := duet && sh.voices[i] == lyricsync.VoiceAll && (i == 0 || sh.voices[i-1] != lyricsync.VoiceAll)
				p.lyricLine(c, sh, &l.Lines[i], i, lineIn{
					sheetIn: in, state: state, prefs: prefs, together: together, textW: textW, reduced: reduced,
					tabbable: i == tabStop || i == sh.want,
				})
			}
		})
	}
}

// lyricGap is the instrumental gap before line i: its dots in the height of a
// line, at the lines' reading edge. Once passed they stay in the column, so
// that nothing moves, but are not drawn and name nothing.
func (p *player) lyricGap(c *ui.Context, sh *lyricsSheet, i int, in sheetIn, state gapState, reduced bool) {
	look := in.look
	row := ui.Box(c.Key("gap:" + strconv.Itoa(i))).FillWidth().Height(look.lineH + 2*look.padY).Bind(&sh.gapAt[i])
	if state == gapPast {
		return
	}
	sh.glide(row, i, reduced)
	row.Role(ui.RoleImage).Label(gapLabel()).
		Draw(p.gapDraw(sh.songID, sh.gaps[i], dotsFor(look.size), look.padX, rtl(sh.lyr.Lines[i].Text), reduced, gapTone(), theme.C("amber-ink")))
}

// lyricsPrefsView is the Translation and Pronunciation choices.
type lyricsPrefsView = lyricsync.Prefs

// lineIn is what a line's build needs.
type lineIn struct {
	sheetIn
	state    lineState
	prefs    lyricsPrefsView
	together bool
	textW    float32
	reduced  bool
	tabbable bool
}

// lyricLine is one line: the main vocal, then the background vocals,
// the romanisation and the translation, each on its own line. A synced line
// in Now Playing is a button that seeks to it.
func (p *player) lyricLine(c *ui.Context, sh *lyricsSheet, ln *services.LyricLine, i int, in lineIn) {
	look := in.look
	voice := sh.voices[i]
	other := voice == lyricsync.VoiceTwo
	shown := lyricsync.Shown(ln, in.prefs)
	tone := lineTone(in.state, len(ln.Words) > 0)
	sub := theme.C("ink-muted")
	clickable := in.clickable && sh.synced

	row := ui.Column(c.Key("line:"+strconv.Itoa(i))).FillWidth().Padding(look.padY, look.padX).Bind(&sh.lines[i])
	sh.glide(row, i, in.reduced)
	startPad := look.padX
	if other {
		startPad += look.indent
		row.PaddingStart(startPad)
	}
	if clickable {
		row.Role(ui.RoleButton).Label(ln.Text).Lang(sh.lyr.Language).Cursor(ui.CursorPointer).Radius(theme.Current().Radius["md"])
		if in.state == stActive {
			row.Current(ui.CurrentItem)
		}
		if in.tabbable {
			row.Focusable()
		}
		if row.Hovered() {
			row.Background(theme.C("ink").Alpha(0.05))
		}
		if row.Focused() {
			sh.focused = i
			if sh.want == i {
				sh.want = -1
			}
		}
		p.lineKeys(c, sh, row, i, ln)
	}

	width := in.textW
	if other {
		width -= look.indent
	}
	row.Children(func() {
		if in.together {
			lineText(c.Key("together"), i18n.T("Together"), look.togetherSize, look.togetherSize*1.3, 500, theme.C("ink-muted")).Margin(0, 0, 2, 0)
		}
		p.lyricPart(c.Key("lead"), sh, i, 0, shown.Lead, tone, look, width, other, in.reduced, false)
		if shown.Background.Text != "" || len(shown.Background.Words) > 0 {
			p.lyricPart(c.Key("bg"), sh, i, 1, shown.Background, sub, look, width, other, in.reduced, true)
		}
		if shown.Pronunciation != nil {
			p.lyricPart(c.Key("pron"), sh, i, 2, *shown.Pronunciation, sub, look, width, other, in.reduced, false)
		}
		if shown.Translation != "" {
			t := lineText(c.Key("tr"), shown.Translation, look.subSize, look.subLineH, 400, sub).Margin(look.subGap, 0, 0, 0).Lang(sh.lyr.TranslationLanguage)
			if other {
				t.Italic()
			}
		}
	})
	if clickable && row.Clicked() {
		sh.userAt = time.Time{}
		p.seekLine(ln)
	}
}

// lineKeys moves the focus between synced lines: one Tab stop, the arrows
// inside; Home and End go to the ends. Enter and Space seek.
func (p *player) lineKeys(c *ui.Context, sh *lyricsSheet, row ui.Element, i int, ln *services.LyricLine) {
	if !row.Focused() {
		return
	}
	last := len(sh.lines) - 1
	to := -1
	switch {
	case row.Shortcut(0, ui.KeyDown):
		to = min(i+1, last)
	case row.Shortcut(0, ui.KeyUp):
		to = max(i-1, 0)
	case row.Shortcut(0, ui.KeyHome):
		to = 0
	case row.Shortcut(0, ui.KeyEnd):
		to = last
	case row.Shortcut(0, ui.KeyEnter), row.Shortcut(0, ui.KeySpace):
		p.seekLine(ln)
	}
	if to >= 0 {
		sh.userScrolled(p.now())
		sh.want, sh.focused = to, to
		sh.lines[to].Focus(c)
	}
}

// lyricPart is one text of a line: word-timed texts are a sweep painted from
// the clock; others plain text.
func (p *player) lyricPart(c *ui.Context, sh *lyricsSheet, i int, part uint8, t lyricsync.Text, base ui.Color, look lyricLook, width float32, italic, reduced, bg bool) {
	size, lineH, weight := look.size, look.lineH, look.weight
	margin := float32(0)
	if part != 0 {
		size, lineH, weight, margin = look.subSize, look.subLineH, 400, look.subGap
		if bg {
			weight = 600
		}
	}
	w := float32(math.Floor(float64(width/sweepStep))) * sweepStep
	if len(t.Words) == 0 || !sh.wordTimed || w <= 0 {
		e := lineText(c, t.Text, size, lineH, weight, base).Margin(margin, 0, 0, 0).Lang(sh.partLang(part))
		if italic {
			e.Italic()
		}
		return
	}
	key := sweepKey{line: i, part: part, italic: italic, whole: reduced}
	sw := sh.sweep(key, t.Words, look, bg, w)
	ui.Box(c).FillWidth().Height(sw.Height()).Margin(margin, 0, 0, 0).Role(ui.RoleText).Label(t.Text).Lang(sh.partLang(part)).
		Draw(p.sweepDraw(sw, sh.songID, sh.spans, i, t, base, theme.C("amber-ink"), reduced))
}

// sweepDraw paints a word-timed text of line i from the clock: each word
// that is sung in fill over base, lifting as it begins and settling back
// when the line stops being sung (kit.SweepText). The sweep paints without
// building the view, and asks for frames only while a word is filling,
// lifting, swelling or settling. Between words, after the last one and before
// the first it sleeps until the next change: a word beginning or ending, the
// line ending or the voice resting. Under reduced motion, or while nothing
// plays, it asks for nothing, and nothing lifts.
func (p *player) sweepDraw(sw *kit.SweepText, songID string, spans []lyricsync.Span, i int, t lyricsync.Text, base, fill ui.Color, reduced bool) func(*ui.Painter, ui.Rect) {
	right := rtl(t.Text)
	until := kit.NoUntil
	if u := lyricsync.SungUntil(spans, i); u != lyricsync.Never {
		until = lyricsync.Dur(u)
	}
	return func(pt *ui.Painter, r ui.Rect) {
		ms := lyricsync.WordClock(p.env.Play.Position().Milliseconds(), lyricsync.Session().Get(songID))
		var f float32
		if lyricsync.IsSinging(spans, ms, i) {
			f = 1
		}
		x := r.X
		if right {
			x = r.X + r.W - sw.Width()
		}
		moving := sw.Paint(pt, x, r.Y, base, fill, kit.SweepClock{T: lyricsync.Dur(ms), Sung: f, Until: until, Still: reduced}, right)
		if reduced || p.env.Play.State().State != "playing" {
			return
		}
		if moving {
			kit.AnimationFrames(pt)
		} else if next := nextSweepEdge(spans, i, t.Words, ms); next != lyricsync.Never {
			pt.After(lyricsync.Dur(next - ms + 1))
		}
	}
}

// lyricsFooter is the credits under the lines: "Written by" and
// "Lyrics from", in the sheet.
func (p *player) lyricsFooter(c *ui.Context, sh *lyricsSheet) {
	l := sh.lyr
	writers := ""
	for i, w := range l.Songwriters {
		if i > 0 {
			writers += ", "
		}
		writers += w
	}
	source := lyricsSourceName(l.Provider)
	muted := theme.C("ink-muted")
	sub := theme.Current().Type["sub"]
	if writers != "" {
		ui.Text(c.Key("writers"), i18n.T("Written by {writers}", "writers", writers)).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
			TextColor(muted).Padding(0, 12).Margin(40, 0, 0, 0)
	}
	if source != "" {
		top := float32(40)
		if writers != "" {
			top = 4
		}
		ui.Text(c.Key("source"), i18n.T("Lyrics from {source}", "source", source)).FontSize(sub.Size).FixedLineHeight(sub.LineHeight).
			TextColor(muted).Padding(0, 12).Margin(top, 0, 0, 0)
	}
}

// lyricsSourceName names a provider, "" for one this version does not know.
func lyricsSourceName(provider string) string {
	switch provider {
	case "apple":
		return i18n.T("Apple Music")
	case "musixmatch":
		return i18n.T("Musixmatch")
	case "lrclib":
		return i18n.T("LRCLIB")
	case "netease":
		return i18n.T("NetEase Cloud Music")
	}
	return ""
}

// seekLine seeks to where the line is the one sung under the song's offset;
// a failure toasts "Couldn't seek".
func (p *player) seekLine(ln *services.LyricLine) {
	songID := lyricsync.SongID(p.env.Play.State().Current)
	off := lyricsync.Session().Get(songID)
	p.tr.SeekTo(lyricsync.Dur(lyricsync.SeekTarget(ln.BeginMs, off)), p.now())
}

// nextSweepEdge is the next time after t at which part of line i could change:
// one of its words beginning or ending, the line ending, the voice resting
// after it, or the line no longer being sung, from where its words settle
// back (lyricsync.Never: none left).
func nextSweepEdge(spans []lyricsync.Span, i int, words []services.LyricWord, t int64) int64 {
	next := lyricsync.Never
	consider := func(ms int64) {
		if ms > t && ms < next {
			next = ms
		}
	}
	for _, w := range words {
		consider(w.BeginMs)
		consider(w.EndMs)
	}
	consider(spans[i].EndMs)
	consider(lyricsync.RestFrom(spans, i))
	consider(lyricsync.SungUntil(spans, i))
	return next
}
