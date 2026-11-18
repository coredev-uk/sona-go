package player

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Full-Screen Lyrics is the Layer slot of the player. Its identifiers keep
// the name of the view it replaced ("room": overlayRoom, roomLayer, and the
// shortcut ID player.lyrics-room, which a user's own binding may be saved
// under); what the listener sees is called Full-Screen Lyrics.

const (
	// idleAfter is how long the pointer and the keys stay still before the
	// foot strip and the pointer go, while the song plays.
	idleAfter = 3 * time.Second
	// stageSeek is Left and Right in the view: a shorter step than the
	// player's Shift+arrows.
	stageSeek = 5 * time.Second
)

// roomOpen reports Full-Screen Lyrics showing: the shell's Layer is open,
// and the window behind it inert.
func (p *player) roomOpen() bool {
	return p.ui.overlay == overlayRoom && lyricsync.SongID(p.env.Play.State().Current) != ""
}

// roomLayer is Full-Screen Lyrics for the shell's Layer slot: the whole
// window given to the song's phrasing over the cover's wash, the words in
// the colours of the lyric sheet, and a foot strip (the song, its progress,
// the timing nudge and Exit full screen) that is there only for a moment
// after the pointer moves or a key is pressed. Escape returns to Now
// Playing. The window goes full screen while it is open (and back when it
// closes, if it was not already), and the screen is kept on while the music
// plays.
//
// The view is dark whatever the theme: it is read across a room and left on
// a TV, so it keeps the house board's dark form (theme.Scope over the dark
// tokens) in light mode too.
func (p *player) roomLayer(c *ui.Context) {
	cur := p.env.Play.State().Current
	songID := lyricsync.SongID(cur)
	if p.ui.overlay == overlayRoom && songID == "" {
		// Nothing to read once the queue is cleared or the song has no lyrics.
		p.ui.closeOverlay()
		c.Invalidate()
	}
	open := p.roomOpen()
	p.roomFullscreen(open)
	if w := p.env.Window; w != nil {
		w.KeepScreenOn(open && p.env.Play.State().State == store.StatePlaying)
	}
	if !open {
		p.np.roomShown, p.np.stage = false, nil
		return
	}
	first := !p.np.roomShown
	p.np.roomShown = true
	if st := p.np.stage; first || st == nil || st.songID != songID {
		next := newStage(songID)
		if st != nil && !first {
			next.carry(st)
		}
		p.np.stage = next
	}
	theme.Scope(theme.TokensFor(true).Color, func() { p.stageView(c, p.np.stage, cur) })
}

// roomFullscreen puts the window in full screen while the lyrics are open,
// and back on leaving if the window was not full screen before.
func (p *player) roomFullscreen(open bool) {
	w := p.env.Window
	if w == nil {
		return
	}
	switch {
	case open && !p.np.roomEntered:
		p.np.roomEntered = true
		if !w.FullScreen() {
			w.SetFullScreen(true)
			p.np.roomFull = true
		}
	case !open && p.np.roomEntered:
		p.np.roomEntered = false
		if p.np.roomFull {
			p.np.roomFull = false
			w.SetFullScreen(false)
		}
	}
}

// stageView is the view: the wash, what the lyrics are (the cascade, or its
// stand-in), and the strip, over the dark ground. It keeps the idle clock.
func (p *player) stageView(c *ui.Context, st *stage, cur *services.QueueItem) {
	now := c.Now()
	w, h := c.Size()
	g := newStageGeom(w, h)
	reduced := theme.ReducedMotion()
	s := p.env.Play.State()
	playing := s.State == store.StatePlaying

	// The strip is there while paused (or loading, or failed), and for
	// idleAfter after the pointer moved or a key was pressed.
	idle := st.wake.IsZero() || now.Sub(st.wake) >= idleAfter
	if playing && !idle {
		c.After(idleAfter - now.Sub(st.wake))
	}
	shown := !playing || !idle

	root := ui.Box(c.Key("room")).Fill().Background(theme.C("ground")).Role(ui.RoleGroup).Label(i18n.T("Full-Screen Lyrics")).
		Description(i18n.T("Space plays and pauses, Up and Down go to the previous or next line, Left and Right go back or ahead 5 seconds, Home goes to the start, Escape leaves.")).
		TabStop(false).FocusRing(false).AutoFocus()
	if !shown {
		root.Cursor(ui.CursorNone)
	}
	if !reduced {
		m := theme.Current().Motion["backdrop"]
		root.Transition(ui.ElementTransition{Colors: true, Enter: &ui.Motion{}, Duration: m.Duration, Ease: m.Ease})
	}
	// Any pointer move or press, and any key that reaches the view, brings
	// the strip back.
	if x, y, over := root.PointerPosition(); over {
		if st.ptrSeen && (x != st.ptrX || y != st.ptrY) {
			st.wake = now
		}
		st.ptrX, st.ptrY, st.ptrSeen = x, y, true
	}
	root.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputKeyDown, ui.InputPointerDown, ui.InputPointerMove:
			st.wake = ev.Time
		}
		return false
	})
	root.Children(func() {
		p.stageWash(c, st, cur, reduced, now)
		p.stageBody(c, st, cur, g, reduced)
		if shown {
			p.stageStrip(c, st, cur, g, reduced)
		}
	})
	if !shown && st.stripFocused {
		// The strip went with the focus on it: the view takes it back.
		st.stripFocused = false
		root.Focus()
	}
}

// stageWash is the cover's pre-blurred wash (kit.Backdrop) under everything,
// drifting very slowly: it scales and slides a few percent over 40 seconds
// and back. The wash works the pose out in its own paint and asks for the next
// paint only when it has moved by about a device pixel (kit.BackdropDrift), so
// it is a few repaints a second, not one per frame. The drift stops under
// reduced motion and Reduce effects (which draw no wash), on battery
// (kit.MotionPolicy), and, as every animation, while the window is not shown.
func (p *player) stageWash(c *ui.Context, st *stage, cur *services.QueueItem, reduced bool, now time.Time) {
	opts := washOf(cur)
	opts.Scrim = theme.TokensFor(true).Scalar["backdrop-scrim"]
	if reduced || !p.env.Motion.Ambient() {
		st.drifted = false
	} else {
		if !st.drifted {
			st.drifted, st.driftAt = true, now
		}
		opts.Drift = &kit.BackdropDrift{Start: st.driftAt, Period: theme.Current().Motion["drift"].Duration, Scale: 1.08, X: -0.01, Y: -0.015}
	}
	kit.Backdrop(c.Key("wash"), p.env.Art, opts, nil).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(true)
}

// stageBody is the lyrics: the cascade, or what stands for it while they
// load, failed to load or do not exist.
func (p *player) stageBody(c *ui.Context, st *stage, cur *services.QueueItem, g stageGeom, reduced bool) {
	r := p.lyricsFor(st.songID)
	switch {
	case !r.HasData && r.Err == nil:
		p.stageLoading(c, g)
	case lyricsMissing(r):
		p.stageNone(c, cur, g)
	case !r.HasData:
		ui.Column(c.Key("error")).Absolute().Left(g.left).Top(0).Bottom(g.h * (1 - 2*stageFocus)).Justify(ui.Center).MaxWidth(576).Children(func() {
			kit.ErrorBlock(c, i18n.T("Couldn't load the lyrics"), r.Err, kit.ErrorBlockOpts{
				OnRetry: func() { p.retryLyrics(st.songID) }, Retrying: r.Fetching,
			})
		})
	default:
		p.stageSung(c, st, &r.Data, g, reduced)
	}
}

// stageSung works out which line the column is at and builds the cascade.
func (p *player) stageSung(c *ui.Context, st *stage, l *services.Lyrics, g stageGeom, reduced bool) {
	st.load(l)
	st.fit(g, p.prefsFor().get(), reduced)
	n := len(l.Lines)
	s := p.env.Play.State()
	playing := s.State == store.StatePlaying
	posMs := p.env.Play.Position().Milliseconds()
	off := lyricsync.Session().Get(st.songID)

	sung := lyricsync.Sheet{Active: -1, Anchor: -1, Next: lyricsync.Never}
	clock := lyricsync.LyricsClock(posMs, off)
	if st.synced {
		sung = lyricsync.SungLine(st.spans, clock)
	} else {
		st.cursor = min(max(st.cursor, 0), n-1)
		sung.Anchor = st.cursor
	}
	// A timer waits for the next line or rest instead of polling the position.
	if playing && sung.Next != lyricsync.Never {
		c.After(lyricsync.Dur(max(sung.Next-clock, 0) + 1))
	}
	if playing && reduced && st.wordTimed {
		p.wakeAtWordChange(c, &st.lyricsModel, lyricsync.WordClock(posMs, off))
	}
	p.cascade(c, st, stageIn{g: g, anchor: min(max(sung.Anchor, 0), n-1), active: sung.Active, reduced: reduced})
}

// stageLoading is the layout with flat bars where the lines will be: the
// sung line's in the middle, two before and two after it. Nothing moves.
func (p *player) stageLoading(c *ui.Context, g stageGeom) {
	bars := [...]struct{ size, wide float32 }{{44, 0.34}, {52, 0.46}, {84, 0.62}, {58, 0.5}, {48, 0.4}}
	const mid = 2
	gap := round32(22 * g.scale)
	var tops, heights [len(bars)]float32
	y := float32(0)
	for i, b := range bars {
		heights[i] = round32(b.size * g.scale * 0.6)
		tops[i] = y
		y += heights[i] + gap
	}
	shift := g.focus - (tops[mid] + heights[mid]/2)
	ui.Box(c.Key("loading")).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(true).Role(ui.RoleStatus).Label(i18n.T("Loading lyrics")).
		Children(func() {
			for i, b := range bars {
				a := float32(0.1)
				if i == mid {
					a = 0.18
				}
				ui.Box(c.Key("bar:" + strconv.Itoa(i))).Absolute().Left(g.left).Top(shift + tops[i]).Width(round32(g.colW * b.wide)).Height(heights[i]).
					Radius(heights[i] / 2).Background(theme.C("ink").Alpha(a))
			}
		})
}

// stageNone is a song with no lyrics, or an instrumental: the wash, the
// cover large in the column's place, the title and artist, and a quiet word.
func (p *player) stageNone(c *ui.Context, cur *services.QueueItem, g stageGeom) {
	cover := min(max(round32(0.42*g.h), 200), 640)
	gap := round32(48 * g.scale)
	ui.Row(c.Key("none")).Absolute().Left(g.left).Right(g.left).Top(0).Bottom(g.h * (1 - 2*stageFocus)).AlignItems(ui.Center).Gap(gap).
		Children(func() {
			kit.Artwork(c.Key("cover"), p.env.Art, cur.Art.Large, cover, kit.ArtOpts{}).Shrink(0)
			ui.Column(c.Key("words")).Grow(1).MinWidth(0).Gap(round32(10 * g.scale)).Children(func() {
				name, by, none := round32(max(44*g.scale, 28)), round32(max(26*g.scale, 18)), round32(max(24*g.scale, 17))
				lineText(c.Key("name"), cur.Name, name, round32(name*1.15), 700, theme.C("ink")).Role(ui.RoleHeading)
				if cur.Artist != "" {
					lineText(c.Key("artist"), cur.Artist, by, round32(by*1.25), 500, theme.C("ink-muted"))
				}
				lineText(c.Key("none"), i18n.T("No lyrics for this song"), none, round32(none*1.3), 400, theme.C("ink-muted")).Margin(round32(14*g.scale), 0, 0, 0)
			})
		})
}

// stageStrip is the foot strip, as the verse block's foot: a 1 DIP rule, the
// 56 DIP cover, the title and artists, the elapsed time, an amber bar, the
// time left, the timing nudge and Exit full screen. It fades in and out.
func (p *player) stageStrip(c *ui.Context, st *stage, cur *services.QueueItem, g stageGeom, reduced bool) {
	s := p.env.Play.State()
	playing := s.State == store.StatePlaying
	ss := min(max(g.scale, 1), 2)
	dur := time.Duration(s.DurationMs) * time.Millisecond
	pos := min(p.env.Play.Position(), dur)
	var frac float64
	if dur > 0 {
		frac = float64(pos) / float64(dur)
	}
	if playing {
		c.After(250 * time.Millisecond) // the time and the bar move
	}
	text := func(key, s string, size, h float32, weight int, tone string) ui.Element {
		return lineText(c.Key(key), s, round32(size*ss), round32(h*ss), weight, theme.C(tone)).SingleLine()
	}
	// A dissolve under the strip, so the coming lines do not run into its rule.
	ground := theme.C("ground")
	fade := ui.Box(c.Key("strip-fade")).Absolute().Left(0).Right(0).Bottom(0).Height(round32(260 * ss)).IgnoreInput(true).
		LinearGradient(ui.LinearGradient{From: ground.Alpha(0), To: ground.Alpha(0.92), Angle: 180, End: 0.6})
	strip := ui.Column(c.Key("strip")).Absolute().Left(g.left).Right(g.left).Bottom(0)
	if !reduced {
		m := theme.Current().Motion["backdrop"]
		tr := ui.ElementTransition{Colors: true, Enter: &ui.Motion{}, Exit: &ui.Motion{}, Duration: m.Duration, Ease: m.Ease}
		fade.Transition(tr)
		strip.Transition(tr)
	}
	strip.Role(ui.RoleGroup).Label(i18n.T("Song")).Children(func() {
		ui.Box(c.Key("rule")).FillWidth().Height(1).Shrink(0).Background(theme.C("line"))
		ui.Row(c.Key("foot")).FillWidth().Height(round32(109 * ss)).Gap(round32(18 * ss)).AlignItems(ui.Center).Children(func() {
			kit.Artwork(c.Key("art"), p.env.Art, cur.Art.Small, round32(56*ss), kit.ArtOpts{}).Shrink(0)
			ui.Column(c.Key("song")).Shrink(0).MaxWidth(round32(0.3 * g.w)).Children(func() {
				text("name", cur.Name, 20, 26, 600, "ink")
				text("artist", cur.Artist, 17, 24, 400, "ink-muted")
			})
			ui.Box(c.Key("space")).Grow(1)
			p.lyricsTiming(c, st.songID)
			if dur > 0 {
				text("elapsed", i18n.FormatDuration(pos), 17, 24, 400, "ink-muted").FontFeatures("tnum").Shrink(0)
				kit.ProgressBar(c.Key("progress"), frac, kit.ProgressOpts{
					A11yLabel: i18n.T("Song progress"),
					ValueText: i18n.T("{at} of {total}", "at", i18n.FormatDuration(pos), "total", i18n.FormatDuration(dur)),
					Width:     min(round32(0.15*g.w), round32(420*ss)), Height: 4, Fill: theme.C("amber-ink"),
				}).MinWidth(0)
				text("left", "\u2212"+i18n.FormatDuration(dur-pos), 17, 24, 400, "ink-muted").FontFeatures("tnum").Shrink(0)
			}
			if kit.Button(c.Key("exit"), i18n.T("Exit full screen"), kit.ButtonOpts{
				Variant: kit.Ghost, Size: kit.SizeSM, Icon: "minimize-2", Tip: i18n.T("Exit Full-Screen Lyrics"), Keys: p.env.KeysOf("player.close-overlay"),
			}).Bind(&p.np.roomExit).Clicked() {
				p.ui.closeOverlay()
			}
		})
	})
	st.stripFocused = strip.FocusWithin()
}

// stageScope is the context of Full-Screen Lyrics' own keys (stageShortcuts).
const stageScope = "lyrics-room"

// stageStep goes to the previous (-1) or next (1) line: it seeks to it, where
// the lyrics are timed. From a break the line just sung is the one to step
// from. Plain lyrics move the reading line.
func (p *player) stageStep(dir int) {
	st := p.np.stage
	if st == nil || st.lyr == nil || len(st.lyr.Lines) == 0 {
		return
	}
	n := len(st.lyr.Lines)
	if !st.synced {
		st.cursor = min(max(st.cursor+dir, 0), n-1)
		return
	}
	from := st.active
	if from < 0 {
		from = st.anchor - 1
	}
	p.seekLine(&st.lyr.Lines[min(max(from+dir, 0), n-1)])
}
