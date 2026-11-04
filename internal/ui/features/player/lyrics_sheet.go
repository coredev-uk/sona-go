package player

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The reading band, as fractions of the scroller's height: while the anchor
// line sits inside it the words hold still; once it leaves, the sheet turns
// so the anchor's verse starts at the band's top (useLyricsView.ts).
const (
	bandTop    = 0.14
	bandBottom = 0.72
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

// lyricLook is the type and spacing of a sheet: Now Playing's, or the room's
// (which scales with the window).
type lyricLook struct {
	room              bool
	size, lineH       float32 // the main vocal
	weight            int
	subSize, subLineH float32 // background vocals, pronunciation, translation
	padX, padY        float32
	subGap, verseGap  float32
	indent            float32 // extra start padding of a duet's other voice
	togetherSize      float32
	topPad, botPad    float32 // the sheet's padding, as fractions of its height
	fadeDIP           float32 // the height of the fade at the top and bottom
	columnMax         float32 // 0: the sheet's whole width
	sidePad           float32
}

// sheetLook is the Now Playing sheet: 24/30 type, lines padded 6×12.
func sheetLook() lyricLook {
	return lyricLook{
		size: 24, lineH: 30, weight: 650, subSize: 16, subLineH: 22, padX: 12, padY: 6,
		subGap: 2, verseGap: 32, indent: 36, togetherSize: 12, topPad: bandTop, botPad: 0.70, fadeDIP: 48,
	}
}

// roomLook is full-screen lyrics: clamp(2.25rem, 4.4vw, 5.25rem) bold, a
// column 24ch wide down the middle, 7vw of side padding.
func roomLook(winW, winH float32) lyricLook {
	s := min(max(0.044*winW, 36), 84)
	sub := s / 2
	return lyricLook{
		room: true, size: s, lineH: s * 1.1, weight: 700, subSize: sub, subLineH: sub * 1.2, padY: s * 0.2,
		subGap: s * 0.15, verseGap: s * 0.9, indent: s * 1.5, togetherSize: s * 0.3,
		topPad: 0.12, botPad: 0.70, fadeDIP: 0.12 * winH, columnMax: 24 * s * 0.55, sidePad: 0.07 * winW,
	}
}

// sweepKey names one shaped text of a line.
type sweepKey struct {
	line   int
	part   uint8 // 0 lead, 1 background, 2 pronunciation
	width  int
	italic bool
	whole  bool
}

// lyricsSheet is one lyric sheet's state: the scroller, the reading band's
// bookkeeping, the keyboard focus and the shaped sweeps. A new song gets a
// new one (v0.2.0 remounted the pane per song). Main thread only.
type lyricsSheet struct {
	songID string

	scroll ui.ScrollState
	box    ui.Handle
	lines  []ui.Handle
	verses []ui.Handle

	lastY      float32
	wasGliding bool
	userAt     time.Time
	anchored   int
	jumped     bool
	// focused is the line that had the focus last (the roving tab stop), -1
	// before any; want is the line a key asked the focus for.
	focused, want int

	// Derived from the lyrics (bind).
	lyr       *services.Lyrics
	spans     []lyricsync.Span
	voices    []lyricsync.Voice
	verseOf   []int
	verseList [][]int
	synced    bool
	wordTimed bool
	sweeps    map[sweepKey]*kit.SweepText

	// Last frame's picture, for tests and for the reading band.
	active, anchor int
	width          float32
}

func newLyricsSheet(songID string) *lyricsSheet {
	return &lyricsSheet{songID: songID, anchored: -1, focused: -1, want: -1}
}

// bind derives what the sheet needs from the lyrics, once per lyrics.
func (sh *lyricsSheet) bind(l *services.Lyrics) {
	if sh.lyr == l {
		return
	}
	sh.lyr = l
	sh.spans = lyricsync.LineSpans(l.Lines)
	sh.voices = lyricsync.LineVoices(l.Lines)
	sh.synced = lyricsync.IsSynced(l)
	sh.wordTimed = lyricsync.HasWordTiming(l)
	sh.verseList = lyricsync.Verses(l.Lines, sh.spans, sh.synced)
	sh.verseOf = make([]int, len(l.Lines))
	for v, ids := range sh.verseList {
		for _, i := range ids {
			sh.verseOf[i] = v
		}
	}
	sh.lines = make([]ui.Handle, len(l.Lines))
	sh.verses = make([]ui.Handle, len(sh.verseList))
	sh.sweeps = map[sweepKey]*kit.SweepText{}
}

// sweep is the shaped text of one part of line i for width, shaped again
// only for a new width.
func (sh *lyricsSheet) sweep(k sweepKey, words []services.LyricWord, look lyricLook, bg bool, w float32) *kit.SweepText {
	if s := sh.sweeps[k]; s != nil {
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
	var s *kit.SweepText
	if k.italic {
		s = kit.NewSweepTextItalic(ws, size, weight, lineH, w)
	} else {
		s = kit.NewSweepText(ws, size, weight, lineH, w)
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
	// clickable makes synced lines buttons that seek (Now Playing's; the
	// room's lines are paragraphs).
	clickable bool
	// focusable makes a synced sheet itself a tab stop (the room: arrow keys scroll it).
	focusable bool
}

// lyricsScroller builds the sheet: the lines in verses inside a scroller that
// turns the page to keep the sung line in the reading band, and fades out at
// its top and bottom. It returns the scroller.
func (p *player) lyricsScroller(c *ui.Context, sh *lyricsSheet, l *services.Lyrics, in sheetIn) ui.Element {
	sh.bind(l)
	look := in.look
	prefs := p.np.prefs.get()
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
		c.After(msDur(max(sung.Next-clock, 0) + 1))
	}
	if playing && reduced && sh.wordTimed {
		p.wakeAtWordChange(c, sh, lyricsync.WordClock(posMs, off))
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
	if !sh.synced || in.focusable {
		// Plain lyrics hold no buttons: the sheet itself takes focus to scroll.
		sheet.Focusable()
	}
	view := sheet.Bounds()
	sh.width = view.W
	text := view.W - 2*look.padX - 2*look.sidePad
	if look.columnMax > 0 {
		text = min(text, look.columnMax)
	}
	sheet.Children(func() {
		col := ui.Column(c.Key("sheet")).FillWidth().Padding(view.H*look.topPad, look.sidePad, view.H*look.botPad)
		if look.columnMax > 0 {
			col.AlignItems(ui.Center)
		}
		col.Children(func() {
			inner := ui.Column(c.Key("column")).FillWidth()
			if look.columnMax > 0 {
				inner.MaxWidth(look.columnMax)
			}
			inner.Children(func() {
				p.lyricVerses(c, sh, in, prefs, sung, text, reduced)
				p.lyricsFooter(c, sh, look)
			})
		})
	})

	// Turn the page: keep the anchor line inside the reading band.
	sh.turnPage(c, view, sung.Anchor, reduced, now)
	sh.lastY = sh.scroll.Y

	// Fade the sheet out at its top and bottom (.lyrics-fade): no blur.
	if view.H > 0 {
		solid, clear := theme.C("ground"), theme.C("ground").Alpha(0)
		fade := min(look.fadeDIP, view.H/3)
		ui.Box(c.Key("fade:top")).Absolute().Top(0).Left(0).Right(0).Height(fade).IgnoreInput(true).Gradient(solid, clear, 180)
		ui.Box(c.Key("fade:bottom")).Absolute().Bottom(0).Left(0).Right(0).Height(fade).IgnoreInput(true).Gradient(clear, solid, 180)
	}
	return sheet
}

// wakeAtWordChange asks for a frame when the next word begins or ends: under
// reduced motion words light whole, and nothing animates in between.
func (p *player) wakeAtWordChange(c *ui.Context, sh *lyricsSheet, t int64) {
	singing := lyricsync.SingingLines(sh.spans, t)
	var words []lyricsync.Span
	for _, i := range singing {
		for _, w := range sh.lyr.Lines[i].Words {
			words = append(words, lyricsync.Span{BeginMs: w.BeginMs, EndMs: w.EndMs})
		}
	}
	if next := lyricsync.NextChange(sh.spans, singing, words, t); next != lyricsync.Never {
		c.After(msDur(next - t + 1))
	}
}

// turnPage keeps the anchor line in view by turning the sheet a verse at a
// time instead of following every line: the words stay still while they are
// read. A turn brings the anchor's verse to the top of the reading band, or
// the anchor itself when its verse is too long for the band. The first turn
// (and every turn under reduced motion) jumps; later ones scroll smoothly.
func (sh *lyricsSheet) turnPage(c *ui.Context, view ui.Rect, anchor int, reduced bool, now time.Time) {
	if anchor < 0 || anchor >= len(sh.lines) || view.H <= 0 || sh.anchored == anchor {
		return
	}
	if now.Sub(sh.userAt) < userScroll {
		return
	}
	lb := sh.lines[anchor].Bounds(c)
	if lb.H <= 0 {
		return // not laid out yet
	}
	sh.anchored = anchor
	h := view.H
	y := sh.scroll.Y
	top := lb.Y - view.Y + y
	bottom := top + lb.H
	jump := !sh.jumped || reduced
	sh.jumped = true
	if !jump && top >= y+h*bandTop-1 && bottom <= y+h*bandBottom {
		return
	}
	verseTop := top
	if v := sh.verseOf[anchor]; v < len(sh.verses) {
		if vb := sh.verses[v].Bounds(c); vb.H > 0 {
			verseTop = vb.Y - view.Y + y
		}
	}
	from := top
	if bottom-verseTop <= h*(bandBottom-bandTop) {
		from = verseTop
	}
	target := max(0, from-h*bandTop)
	if jump {
		sh.scroll.Y = target
	} else {
		sh.scroll.ScrollTo(0, target, ui.Smooth)
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
		verse := ui.Column(c.Key("verse:" + itoa(v))).FillWidth().Bind(&sh.verses[v])
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
				together := duet && sh.voices[i] == lyricsync.VoiceAll && (i == 0 || sh.voices[i-1] != lyricsync.VoiceAll)
				p.lyricLine(c, sh, &l.Lines[i], i, lineIn{
					sheetIn: in, state: state, prefs: prefs, together: together, textW: textW, reduced: reduced,
					tabbable: i == tabStop || i == sh.want,
				})
			}
		})
	}
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
	if look.room && in.state == stPast {
		sub = theme.C("ink-faint")
	}
	clickable := in.clickable && sh.synced

	row := ui.Column(c.Key("line:"+itoa(i))).FillWidth().Padding(look.padY, look.padX).Bind(&sh.lines[i])
	startPad := look.padX
	if other {
		startPad += look.indent
		row.PaddingStart(startPad)
	}
	if clickable {
		row.Role(ui.RoleButton).Label(ln.Text).Cursor(ui.CursorPointer).Radius(theme.Current().Radius["md"])
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
			t := lineText(c.Key("tr"), shown.Translation, look.subSize, look.subLineH, 400, sub).Margin(look.subGap, 0, 0, 0)
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
		e := lineText(c, t.Text, size, lineH, weight, base).Margin(margin, 0, 0, 0)
		if italic {
			e.Italic()
		}
		return
	}
	key := sweepKey{line: i, part: part, width: int(w), italic: italic, whole: reduced}
	sw := sh.sweep(key, t.Words, look, bg, w)
	right := rtl(t.Text)
	fill := theme.C("amber-ink")
	songID, spans := sh.songID, sh.spans
	sp := spans[i]
	ui.Box(c).FillWidth().Height(sw.Height()).Margin(margin, 0, 0, 0).Role(ui.RoleText).Label(t.Text).Draw(func(pt *ui.Painter, r ui.Rect) {
		ms := lyricsync.WordClock(p.env.Play.Position().Milliseconds(), lyricsync.Session().Get(songID))
		singing := lyricsync.IsSinging(spans, ms, i)
		var f float32
		if singing {
			f = 1
		}
		x := r.X
		if right {
			x = r.X + r.W - sw.Width()
		}
		moving := sw.Paint(pt, x, r.Y, base, fill, f, msDur(ms), right)
		// The sweep paints from the clock at the display's rate, without
		// building the view: a frame is asked for only while a word is
		// being sung (or a line is lit and a next word is due).
		// A line is about to be sung from the lead before its first word: the
		// view is built at the line, the first word comes a lead later.
		near := ms >= sp.BeginMs-2*lyricsync.LeadMs && ms < sp.EndMs
		if p.env.Play.State().State == "playing" && !reduced && (moving || singing || near) {
			sweepFrames(pt)
		}
	})
}

// lyricsFooter is the credits under the lines: "Written by" and
// "Lyrics from", in the sheet.
func (p *player) lyricsFooter(c *ui.Context, sh *lyricsSheet, look lyricLook) {
	if look.room {
		return
	}
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
	songID := lyricsSongID(p.env.Play.State().Current)
	off := lyricsync.Session().Get(songID)
	p.seekTo(msDur(lyricsync.SeekTarget(ln.BeginMs, off)))
}
