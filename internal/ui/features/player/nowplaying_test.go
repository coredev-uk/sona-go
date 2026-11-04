package player

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// seeker records what Now Playing asked of the playback service; the rest of
// the transport is nil (a call would panic: Now Playing makes none).
type seeker struct {
	transport
	seeks chan int64
}

func (s *seeker) Seek(_ context.Context, ms int64) error {
	s.seeks <- ms
	return nil
}

// fakeLyrics answers the lyrics query.
type fakeLyrics struct {
	mu    sync.Mutex
	l     services.Lyrics
	err   error
	calls int
}

func (f *fakeLyrics) Lyrics(_ context.Context, _ string) (services.Lyrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.l, f.err
}

func (f *fakeLyrics) set(l services.Lyrics, err error) {
	f.mu.Lock()
	f.l, f.err = l, err
	f.mu.Unlock()
}

func (f *fakeLyrics) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// timedLines are n lines four seconds apart from 1 s, each sung for three,
// "Line i sung words" in four words of 750 ms when word-timed.
func timedLines(n int, words bool) []services.LyricLine {
	out := make([]services.LyricLine, n)
	for i := range out {
		begin := int64(1000 + i*4000)
		ln := services.LyricLine{BeginMs: begin, EndMs: begin + 3000, Text: fmt.Sprintf("Line %d sung words", i), Agent: "v1", AgentType: "person"}
		if words {
			for k, w := range []string{"Line ", fmt.Sprintf("%d ", i), "sung ", "words"} {
				b := begin + int64(k)*750
				ln.Words = append(ln.Words, services.LyricWord{BeginMs: b, EndMs: b + 750, Text: w})
			}
		}
		out[i] = ln
	}
	return out
}

type npRig struct {
	p    *player
	tt   *uitest.Session
	seek *seeker
	lyr  *fakeLyrics
	h    *uitest.Harness
	// width and height are the window's size (1132×700 when zero).
	width, height int
}

func newNPRig(t *testing.T, mutate ...func(*npRig)) *npRig {
	t.Helper()
	return newNPRigWith(t, nil, mutate...)
}

func newNPRigWith(t *testing.T, opts []uitest.Option, mutate ...func(*npRig)) *npRig {
	t.Helper()
	h := uitest.New(t, append([]uitest.Option{uitest.Bare()}, opts...)...)
	r := &npRig{h: h, seek: &seeker{seeks: make(chan int64, 8)}, lyr: &fakeLyrics{}}
	r.p = newPlayer(h.Env, r.seek, func() bool { return false })
	r.p.now = h.Clock.Now
	r.p.np.prefsInit = true // the choices stay in memory
	r.p.np.lyrics = r.lyr
	lyricsync.Session().Clear()
	t.Cleanup(lyricsync.Session().Clear)
	for _, m := range mutate {
		m(r)
	}
	if r.width == 0 {
		r.width, r.height = 1132, 700
	}
	r.tt = h.Tester(t, func(c *ui.Context) {
		// A frozen fake clock never finishes a transition: every input would
		// draw twenty frames.
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
				if slot := r.p.overlaySlot(); slot.Open() {
					slot.Build(c)
				}
			})
		})
		// The shell's layer wrapper: input only while the layer is open.
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(!r.p.roomOpen()).Children(func() { r.p.roomLayer(c) })
	}, r.width, r.height)
	return r
}

// play shows a playing catalogue song at position ms.
func (r *npRig) play(ms int64, mutate ...func(*services.PlaybackState)) {
	s := store.EmptyState()
	s.State = store.StatePlaying
	s.Current = &services.QueueItem{
		ID: "i.1", CatalogID: "1001", Name: "Song Title", Artist: "The Artist", Album: "The Album",
		ArtistID: "a1", AlbumID: "al1", Explicit: true,
	}
	s.DurationMs, s.PositionMs, s.At = 240_000, ms, r.h.Clock.Now().UnixMilli()
	s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true}
	for _, m := range mutate {
		m(&s)
	}
	r.h.Env.Play.Seed(s, services.QueueState{Index: -1})
}

// advance moves the song and the tester's clock on by d, drawing frames.
func (r *npRig) advance(d time.Duration) {
	r.h.Clock.Advance(d)
	r.tt.Advance(d)
	r.tt.Frame()
}

func (r *npRig) open(pane paneKind) {
	r.p.ui.openNowPlaying(pane)
	r.tt.Until("Now Playing is up", func() bool { return r.tt.HasText("Now playing") })
}

func (r *npRig) settle() {
	for range 4 {
		r.tt.Drain()
	}
}

func TestNowPlayingLayoutFollowsItsBoxNotTheWindow(t *testing.T) {
	for _, c := range []struct {
		w, h    float32
		stacked bool
		cover   float32
	}{
		{879, 900, true, 168}, // narrow: the song becomes a header
		{880, 900, false, 320},
		{1000, 900, false, 400},
		{1000, 720, false, 400},
		{1000, 719, false, 320},
		{1000, 640, false, 320},
		{1000, 639, false, 240},
		{900, 900, false, 320},
	} {
		got := nowPlayingLayoutFor(c.w, c.h)
		if got.stacked != c.stacked || got.cover != c.cover {
			t.Errorf("%v×%v: %+v, want stacked=%v cover=%v", c.w, c.h, got, c.stacked, c.cover)
		}
	}
}

func TestMotionCoverIsAskedForOnlyWhileMotionMayPlay(t *testing.T) {
	inBubble(t, motionCoverIsAskedForOnlyWhileMotionMayPlay)
}

func motionCoverIsAskedForOnlyWhileMotionMayPlay(t *testing.T) {
	asked := make(chan string, 4)
	r := newNPRig(t, func(r *npRig) { r.p.np.motion = motionFunc(func(id string) { asked <- id }) })
	// No policy (a hand-built Env): motion never plays and nothing is asked.
	if got := r.p.albumMotion("al1"); got != "" {
		t.Errorf("motion %q without a policy", got)
	}
	synctest.Wait() // a goroutine that was going to ask has asked by now
	select {
	case id := <-asked:
		t.Errorf("the motion service was asked for %q while motion is not allowed", id)
	default:
	}
	// No album, no ask either way.
	if got := r.p.albumMotion(""); got != "" {
		t.Errorf("motion %q for no album", got)
	}
}

// motionOpens records the motion files the policy was asked to open; every open
// fails, so the still stays.
type motionOpens struct {
	mu    sync.Mutex
	calls []string
}

func (m *motionOpens) open(_ context.Context, url string, px int) (*ui.VideoPlayer, error) {
	m.mu.Lock()
	m.calls = append(m.calls, fmt.Sprintf("%s@%d", url, px))
	m.mu.Unlock()
	return nil, errors.New("no decoder in a test")
}

func (m *motionOpens) opened() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.calls)
}

func TestNowPlayingCoverAsksForTheAlbumMotionWhenMotionIsAllowed(t *testing.T) {
	asked := make(chan string, 4)
	fake := &motionOpens{}
	r := newNPRig(t, func(r *npRig) {
		r.p.np.motion = motionFunc(func(id string) { asked <- id })
		pol := kit.NewTestMotionPolicy(r.h.Main.Post, fake.open)
		t.Cleanup(pol.Close)
		r.h.Env.Motion = pol
	})
	r.play(2000)
	r.open("")
	r.tt.Until("the cover asks for the album's motion file", func() bool { return len(fake.opened()) > 0 })
	select {
	case id := <-asked:
		if id != "al1" {
			t.Errorf("the motion service was asked for album %q", id)
		}
	default:
		t.Error("the motion service was never asked")
	}
	got := fake.opened()
	if !strings.HasPrefix(got[0], "/motion/x/1.mp4@") {
		t.Errorf("the cover opened %q, want the album's motion file", got[0])
	}
}

type motionFunc func(id string)

func (f motionFunc) Album(_ context.Context, id string) (string, error) {
	f(id)
	return "/motion/x/1.mp4", nil
}

func TestPanesFallBackToUpNextWhereThereAreNoLyrics(t *testing.T) {
	song := &services.QueueItem{CatalogID: "1"}
	for name, c := range map[string]struct {
		cur  *services.QueueItem
		want []paneKind
	}{
		"song":      {song, []paneKind{paneLyrics, paneQueue}},
		"station":   {&services.QueueItem{CatalogID: "1", Live: true}, []paneKind{paneQueue}},
		"video":     {&services.QueueItem{CatalogID: "1", Video: true}, []paneKind{paneQueue}},
		"library":   {&services.QueueItem{LibraryID: "i.9"}, []paneKind{paneQueue}},
		"no item":   {nil, []paneKind{paneQueue}},
		"live song": {&services.QueueItem{Live: true}, []paneKind{paneQueue}},
	} {
		got := panesFor(c.cur)
		if len(got) != len(c.want) || got[0] != c.want[0] {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
		if name != "song" && shownPane(paneLyrics, c.cur) != paneQueue {
			t.Errorf("%s shows %v", name, shownPane(paneLyrics, c.cur))
		}
	}
}

func TestNowPlayingShowsTheSongTabsAndTheLyricSheet(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Provider: "apple", Timing: "Line", Songwriters: []string{"A Writer", "B Writer"}, Lines: timedLines(30, false)}, nil)
	r.play(5000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 1 sung words"))
	for _, w := range []string{"Song Title", "The Artist", "The Album", "Lyrics", "Up next", "Close now playing (Escape)", "Explicit", "Full screen", "Lyrics on time", "Line 0 sung words"} {
		if !r.tt.HasText(w) {
			t.Errorf("Now Playing lacks %q: %q", w, r.tt.Texts())
		}
	}
	if r.p.np.sheet == nil || r.p.np.sheet.active != 1 {
		t.Errorf("the sung line is %v at 5 s", r.p.np.sheet.active)
	}
	// Written by and the source close the sheet; scroll down to see them.
	r.p.np.sheet.scroll.Y = r.p.np.sheet.scroll.MaxY
	r.advance(10 * time.Millisecond)
	for _, w := range []string{"Written by A Writer, B Writer", "Lyrics from Apple Music"} {
		if !r.tt.HasText(w) {
			t.Errorf("the sheet's foot lacks %q", w)
		}
	}
}

func TestLyricsLoadingNotFoundAndFailure(t *testing.T) {
	r := newNPRig(t)
	gate := make(chan struct{})
	r.p.np.lyrics = gatedLyrics{r.lyr, gate}
	r.lyr.set(services.Lyrics{}, errors.New("not found"))
	r.play(0)
	r.open("")
	if !r.tt.HasText("Loading lyrics") {
		t.Errorf("no loading state: %q", r.tt.Texts())
	}
	// A failure that is not "not found": the error block, with a retry that asks again.
	r.lyr.set(services.Lyrics{}, &url.Error{Op: "Get", URL: "https://lyrics.example", Err: errors.New("connection refused")})
	close(gate)
	r.tt.Until("failure", r.tt.Shows("Couldn't load the lyrics", "Try again"))
	before := r.lyr.count()
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(3, false)}, nil)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("lyrics after the retry", r.tt.Shows("Line 1 sung words"))
	if r.lyr.count() <= before {
		t.Error("Try again did not ask the service")
	}

	// A song none of the sources has: the empty card, once.
	r2 := newNPRig(t)
	r2.lyr.set(services.Lyrics{}, lyrics.ErrNotFound)
	r2.play(0)
	r2.open("")
	r2.tt.Until("no lyrics", r2.tt.Shows("No lyrics for this song", "None of the lyrics sources in Settings has lyrics for it."))
	n := r2.lyr.count()
	r2.advance(10 * time.Second)
	if r2.lyr.count() != n {
		t.Errorf("a song without lyrics was asked again: %d → %d", n, r2.lyr.count())
	}
}

type gatedLyrics struct {
	*fakeLyrics
	gate chan struct{}
}

func (g gatedLyrics) Lyrics(ctx context.Context, id string) (services.Lyrics, error) {
	<-g.gate
	return g.fakeLyrics.Lyrics(ctx, id)
}

func TestTheSheetFollowsTheSungLine(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(60, false)}, nil)
	r.play(2000) // line 0 began at 1 s
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	r.settle()
	sh := r.p.np.sheet
	if sh.active != 0 || sh.scroll.Y != 0 {
		t.Fatalf("at the start: active %d, scrolled %v", sh.active, sh.scroll.Y)
	}
	// The voice moves on: the sung line follows the clock under the lead, one
	// line at a time, and the sheet holds still while it stays in the band.
	r.play(5300) // 5.3 s + 250 ms lead: line 1 (begins 5 s)
	r.settle()
	if sh.active != 1 {
		t.Errorf("active %d at 5.3 s, want 1", sh.active)
	}
	// Far on: the sheet turned (smoothly), the anchor line is in view.
	r.play(1000 + 40*4000 + 500)
	for range 40 {
		r.advance(50 * time.Millisecond)
	}
	if sh.active < 40 || sh.scroll.Y <= 0 {
		t.Errorf("at line 40: active %d, scrolled %v", sh.active, sh.scroll.Y)
	}
	if !r.tt.HasText(fmt.Sprintf("Line %d sung words", sh.active)) {
		t.Errorf("the sung line %d is not in view: %q", sh.active, r.tt.Texts())
	}
	// A seek back: the page turns back to the top.
	r.play(500)
	for range 60 {
		r.advance(50 * time.Millisecond)
	}
	if sh.active != 0 || sh.scroll.Y > 1 {
		t.Errorf("after a seek back: active %d, scrolled %v", sh.active, sh.scroll.Y)
	}
}

func TestAUserScrollLeavesTheSheetAloneForFourSeconds(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(60, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	r.settle()
	sh := r.p.np.sheet
	// The user scrolls the sheet down.
	sh.scroll.Y = 400
	r.advance(time.Millisecond)
	if sh.userAt.IsZero() {
		t.Fatal("the scroll was not noticed")
	}
	y := sh.scroll.Y
	// The song moves to a line far outside the band, but the sheet is left alone.
	r.play(1000 + 30*4000 + 500)
	r.advance(100 * time.Millisecond)
	if sh.scroll.Y != y {
		t.Errorf("the sheet moved from %v to %v inside the 4 s", y, sh.scroll.Y)
	}
	// After the 4 s the next change of line brings it back.
	r.advance(4 * time.Second)
	r.play(1000 + 31*4000 + 500)
	for range 40 {
		r.advance(50 * time.Millisecond)
	}
	if sh.scroll.Y == y {
		t.Errorf("the sheet stayed at %v after the 4 s", y)
	}
}

func TestClickingALineSeeksToItsBeginPlusTheOffset(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(30, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 3 sung words"))
	lyricsync.Session().Nudge("1001", 1) // lyrics half a second later
	r.advance(time.Millisecond)
	if err := r.tt.Click("Line 3 sung words"); err != nil {
		t.Fatal(err)
	}
	select {
	case ms := <-r.seek.seeks:
		if want := int64(1000+3*4000) + 500; ms != want {
			t.Errorf("seeked to %d, want %d", ms, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clicking a line did not seek")
	}
	// The mirror shows the new position at once, until Go's next snapshot.
	if got := r.h.Env.Play.State().PositionMs; got != 13_500 {
		t.Errorf("the mirror is at %d", got)
	}
}

func TestPlainLyricsAreNotButtons(t *testing.T) { inBubble(t, plainLyricsAreNotButtons) }

func plainLyricsAreNotButtons(t *testing.T) {
	r := newNPRig(t)
	lines := timedLines(5, false)
	r.lyr.set(services.Lyrics{Timing: "None", Lines: lines}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 3 sung words"))
	_ = r.tt.Click("Line 3 sung words")
	r.tt.Quiesce() // a seek that got through has run by now
	select {
	case ms := <-r.seek.seeks:
		t.Errorf("a plain line seeked to %d", ms)
	default:
	}
	if r.tt.HasText("Full screen") || r.tt.HasText("Lyrics on time") {
		t.Error("plain lyrics have timing and full-screen tools")
	}
}

func TestLyricsTimingNudgesPerSongAndResets(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(10, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("tools", r.tt.Shows("Lyrics on time"))
	click := func(s string) {
		t.Helper()
		if err := r.tt.Click(s); err != nil {
			t.Fatal(err)
		}
		r.tt.Drain()
	}
	click("Show lyrics 0.5 s later")
	click("Show lyrics 0.5 s later")
	if got := lyricsync.Session().Get("1001"); got != 1000 || !r.tt.HasText("Lyrics 1 s later") {
		t.Errorf("offset %d: %q", got, r.tt.Texts())
	}
	click("Show lyrics 0.5 s earlier")
	click("Show lyrics 0.5 s earlier")
	click("Show lyrics 0.5 s earlier")
	if got := lyricsync.Session().Get("1001"); got != -500 || !r.tt.HasText("Lyrics 0.5 s earlier") {
		t.Errorf("offset %d: %q", got, r.tt.Texts())
	}
	click("Lyrics 0.5 s earlier") // the readout puts the song back on its timing
	if got := lyricsync.Session().Get("1001"); got != 0 || !r.tt.HasText("Lyrics on time") {
		t.Errorf("offset %d after the reset: %q", got, r.tt.Texts())
	}
	// Another song starts in sync.
	if lyricsync.Session().Get("2002") != 0 {
		t.Error("the offset leaked to another song")
	}
}

func TestTranslationAndPronunciationTogglesAddTheirLines(t *testing.T) {
	r := newNPRig(t)
	lines := timedLines(6, true)
	for i := range lines {
		lines[i].Translation = &services.LyricText{Text: fmt.Sprintf("Translated %d", i)}
		lines[i].Transliteration = &services.LyricText{Text: fmt.Sprintf("Romanised %d", i), Words: lines[i].Words}
	}
	r.lyr.set(services.Lyrics{Timing: "Word", Language: "ko", TranslationLanguage: "en-US", TransliterationLanguage: "ko-Latn", Lines: lines}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("tools", r.tt.Shows("Translation", "Pronunciation"))
	if r.tt.HasText("Translated 1") || r.tt.HasText("Romanised 1") {
		t.Fatal("the localizations show before they are asked for")
	}
	_ = r.tt.Click("Translation")
	r.tt.Drain()
	if !r.tt.HasText("Translated 1") {
		t.Errorf("no translation: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Pronunciation")
	r.tt.Drain()
	if !r.tt.HasText("Romanised 1") {
		t.Errorf("no pronunciation: %q", r.tt.Texts())
	}
	if p := r.p.np.prefs.get(); !p.Translation || !p.Pronunciation {
		t.Errorf("choices %+v", p)
	}
	_ = r.tt.Click("Translation")
	r.tt.Drain()
	if r.tt.HasText("Translated 1") {
		t.Error("the translation stays after it is turned off")
	}
}

func TestLyricsChoicesAreKeptInTheSettings(t *testing.T) {
	h := uitest.New(t)
	until := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
			h.Main.Drain()
			if ok() {
				return
			}
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	a := newPlayer(h.Env, nil, func() bool { return false })
	prefs := a.prefsFor()
	if got := prefs.get(); got != (lyricsync.Prefs{}) {
		t.Errorf("defaults %+v", got)
	}
	// Saved through the settings, off the main thread; shown at once.
	until("the settings", func() bool { return store.UseSettings(h.Env.Queries, h.Env.Core).HasData })
	prefs.set(lyricsync.Prefs{Translation: true})
	if got := prefs.get(); !got.Translation || got.Pronunciation {
		t.Errorf("not shown at once: %+v", got)
	}
	sys := services.Service[services.SystemService](h.Core)
	until("the save", func() bool { return sys.Settings().LyricsView.Translation })
	// After a restart (a fresh cache over the same settings) they are still on.
	env := *h.Env
	env.Queries = store.NewClient(h.Main.Post)
	b := newPlayer(&env, nil, func() bool { return false })
	until("the choices after a restart", func() bool { return b.prefsFor().get().Translation })
}

func TestKeyboardMovesBetweenSyncedLines(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(12, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 3 sung words"))
	r.settle()
	// The tab stop is the sung line; the arrows move inside the sheet.
	sh := r.p.np.sheet
	sh.lines[0].Focus()
	r.settle()
	r.tt.Key(0, ui.KeyDown)
	r.settle()
	if sh.focused != 1 {
		t.Errorf("focus on line %d after Down, want 1", sh.focused)
	}
	r.tt.Key(0, ui.KeyEnd)
	r.settle()
	if sh.focused != 11 {
		t.Errorf("focus on line %d after End, want 11", sh.focused)
	}
	r.tt.Key(0, ui.KeyHome)
	r.settle()
	if sh.focused != 0 {
		t.Errorf("focus on line %d after Home, want 0", sh.focused)
	}
}

func TestDuetsTogetherCueAndVoices(t *testing.T) {
	r := newNPRig(t)
	lines := timedLines(6, false)
	lines[1].Agent, lines[2].Agent, lines[3].AgentType, lines[3].Agent = "v2", "v2", "group", "v1000"
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: lines}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 3 sung words"))
	if !r.tt.HasText("Together") {
		t.Errorf("no Together cue on the first group line: %q", r.tt.Texts())
	}
}

func TestOpeningRoomLyricsAndLeavingThem(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(30, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("tools", r.tt.Shows("Full screen"))
	if err := r.tt.Click("Full screen"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the room", r.tt.Shows("Lyrics, full screen", "Exit full screen", "Song Title"))
	if r.p.ui.overlay != overlayRoom || !r.p.roomOpen() {
		t.Errorf("overlay %v", r.p.ui.overlay)
	}
	if !r.tt.HasText("Line 0 sung words") || !r.tt.HasText("Lyrics on time") {
		t.Errorf("the room lacks the sheet or the timing: %q", r.tt.Texts())
	}
	// Leaving returns to Now Playing, whose Full screen button has the focus.
	if err := r.tt.Click("Exit full screen"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Now Playing again", r.tt.Shows("Now playing", "Full screen"))
	if r.p.ui.overlay != overlayNowPlaying || !r.p.ui.fromRoom {
		t.Errorf("overlay %v fromRoom %v", r.p.ui.overlay, r.p.ui.fromRoom)
	}
	r.settle()
	if !r.tt.Focused("Full screen") {
		t.Error("focus did not return to Full screen")
	}
	// Close steps out to the page.
	if err := r.tt.Click("Close now playing (Escape)"); err != nil {
		t.Fatal(err)
	}
	if r.p.ui.overlay != overlayNone {
		t.Errorf("overlay %v after Close", r.p.ui.overlay)
	}
}

func TestRoomLyricsCloseWhenTheSongHasNone(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(5, false)}, nil)
	r.play(2000)
	r.p.ui.openRoom()
	r.tt.Until("the room", r.tt.Shows("Lyrics, full screen"))
	// A music video plays: no lyrics id.
	r.play(0, func(s *services.PlaybackState) { s.Current.Video = true })
	r.settle()
	if r.p.ui.overlay == overlayRoom {
		t.Error("the room stays over a song with no lyrics")
	}
}

func TestNowPlayingClosesWhenTheQueueIsCleared(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(5, false)}, nil)
	r.play(2000)
	r.open("")
	r.h.Env.Play.Seed(store.EmptyState(), services.QueueState{Index: -1})
	r.settle()
	if r.p.ui.overlay != overlayNone {
		t.Errorf("overlay %v with nothing playing", r.p.ui.overlay)
	}
}

func TestPaneTabsSwitchAndKeepTheirPane(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(5, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("tabs", r.tt.Shows("Up next"))
	if err := r.tt.Click("Up next"); err != nil {
		t.Fatal(err)
	}
	r.settle()
	if r.p.ui.pane != paneQueue {
		t.Errorf("pane %v after clicking Up next", r.p.ui.pane)
	}
	// Closed and reopened, it reopens on the pane it showed last.
	r.p.ui.closeOverlay()
	r.settle()
	r.p.ui.openNowPlaying("")
	r.settle()
	if r.p.ui.pane != paneQueue {
		t.Errorf("reopened on %v", r.p.ui.pane)
	}
}

func TestShortcutsOpenAndNudge(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(5, false)}, nil)
	byID := map[string]module.Shortcut{}
	for _, s := range r.p.nowPlayingShortcuts() {
		byID[s.ID] = s
	}
	for id, keys := range map[string]string{
		"player.lyrics": "L", "player.now-playing": "N", "player.video": "V", "player.close-overlay": "Escape",
		"player.lyrics-earlier": "[", "player.lyrics-later": "]", "player.lyrics-reset": "\\", "player.lyrics-room": "Shift+L",
	} {
		s, ok := byID[id]
		if !ok || len(s.Keys) != 1 || s.Keys[0] != keys {
			t.Errorf("%s: %+v, want key %s", id, s.Keys, keys)
		}
	}
	if byID["player.close-overlay"].Scope != "now-playing" || !byID["player.close-overlay"].NoPalette {
		t.Error("Escape is the now-playing scope, kept out of the palette")
	}
	if !byID["player.lyrics-earlier"].Repeat || !byID["player.lyrics-later"].Repeat {
		t.Error("the nudges repeat while held")
	}

	// Nothing playing: L and N do nothing.
	byID["player.lyrics"].Run()
	byID["player.now-playing"].Run()
	if r.p.ui.overlay != overlayNone {
		t.Error("opened with nothing playing")
	}
	r.play(0)
	// L opens on Lyrics; again, it closes.
	byID["player.lyrics"].Run()
	if r.p.ui.overlay != overlayNowPlaying || r.p.ui.pane != paneLyrics {
		t.Errorf("L: %v %v", r.p.ui.overlay, r.p.ui.pane)
	}
	if !byID["player.lyrics-earlier"].When() {
		t.Error("the nudge is unavailable while the lyrics are shown")
	}
	byID["player.lyrics-later"].Run()
	byID["player.lyrics-later"].Run()
	byID["player.lyrics-earlier"].Run()
	if got := lyricsync.Session().Get("1001"); got != 500 {
		t.Errorf("offset %d", got)
	}
	byID["player.lyrics-reset"].Run()
	if lyricsync.Session().Get("1001") != 0 {
		t.Error("reset did not put the lyrics back")
	}
	byID["player.lyrics"].Run()
	if r.p.ui.overlay != overlayNone {
		t.Error("L did not close the lyrics")
	}
	if byID["player.lyrics-earlier"].When() {
		t.Error("the nudge is available with the lyrics hidden")
	}
	// N opens on the last pane, Shift+L is the room, Escape steps back a level.
	byID["player.now-playing"].Run()
	byID["player.lyrics-room"].Run()
	if r.p.ui.overlay != overlayRoom {
		t.Errorf("Shift+L: %v", r.p.ui.overlay)
	}
	byID["player.close-overlay"].Run()
	if r.p.ui.overlay != overlayNowPlaying || !r.p.ui.fromRoom {
		t.Errorf("Escape from the room: %v fromRoom %v", r.p.ui.overlay, r.p.ui.fromRoom)
	}
	byID["player.close-overlay"].Run()
	if r.p.ui.overlay != overlayNone {
		t.Errorf("Escape from Now Playing: %v", r.p.ui.overlay)
	}
	// A music video hides L and unhides V.
	r.play(0, func(s *services.PlaybackState) { s.Current.Video = true })
	if byID["player.lyrics"].When() || !byID["player.video"].When() {
		t.Error("L is for songs and V for videos")
	}
	if byID["player.lyrics-room"].When() {
		t.Error("a video has no lyrics for the room")
	}
}

func TestSweepWordsLightWholeUnderReducedMotion(t *testing.T) {
	ws := []services.LyricWord{{BeginMs: 100, EndMs: 900, Text: "a "}, {BeginMs: 900, EndMs: 1500, Text: "b"}}
	for _, w := range sweepWords(ws, false) {
		if w.End <= w.Begin {
			t.Errorf("words keep their length: %v", w)
		}
	}
	for _, w := range sweepWords(ws, true) {
		if w.End != w.Begin {
			t.Errorf("a whole word has no length: %v", w)
		}
	}
	if !rtl("שלום") || rtl("hello") || !rtl("123 مرحبا") || rtl("") {
		t.Error("right-to-left detection")
	}
}

func TestDescribeOffset(t *testing.T) {
	for ms, want := range map[int64]string{0: "", -500: "0.5 s earlier", 2000: "2 s later", 1500: "1.5 s later"} {
		if got := describeOffset(ms); got != want {
			t.Errorf("describeOffset(%d) = %q, want %q", ms, got, want)
		}
	}
}

func TestNowPlayingGolden(t *testing.T) {
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
			if err := theme.RegisterFonts(); err != nil {
				t.Fatal(err)
			}
			var opts []uitest.Option
			if mode.dark {
				opts = append(opts, uitest.Dark())
			}
			r := newNPRigWith(t, opts)
			lines := timedLines(30, true)
			r.lyr.set(services.Lyrics{Provider: "apple", Timing: "Word", Songwriters: []string{"A Writer"}, Lines: lines}, nil)
			r.play(14_500)
			r.open("")
			r.tt.Until("lyrics", r.tt.Shows("Line 3 sung words"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "nowplaying-"+mode.name)
		})
	}
}

// TestNowPlayingEvidence writes the pictures of the PR's description
// (NP_EVIDENCE_DIR): Now Playing on word-timed lyrics with the translation on,
// the empty card and full-screen lyrics, light and dark, at the window's
// default and minimum sizes.
func TestNowPlayingEvidence(t *testing.T) {
	dir := os.Getenv("NP_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("set NP_EVIDENCE_DIR to write the evidence pictures")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	save := func(r *npRig, name string) {
		r.tt.Settle()
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, r.tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"light", "dark"} {
		for _, size := range [][2]int{{1132, 1253}, {940, 600}} {
			label := fmt.Sprintf("%s-%dx%d", mode, size[0], size[1])
			var opts []uitest.Option
			if mode == "dark" {
				opts = append(opts, uitest.Dark())
			}
			size := size
			lines := timedLines(40, true)
			for i := range lines {
				lines[i].Translation = &services.LyricText{Text: fmt.Sprintf("Translated line %d", i)}
			}
			r := newNPRigWith(t, opts, func(r *npRig) { r.width, r.height = size[0], size[1] })
			r.lyr.set(services.Lyrics{Provider: "apple", Timing: "Word", Songwriters: []string{"A Writer"}, TranslationLanguage: "en-US", Lines: lines}, nil)
			r.p.prefsFor().set(lyricsync.Prefs{Translation: true})
			r.play(14_800)
			r.open("")
			r.tt.Until("lyrics", r.tt.Shows("Line 3 sung words"))
			save(r, "now-playing-"+label)
			if size[0] == 1132 {
				r.p.ui.openRoom()
				r.tt.Until("the room", r.tt.Shows("Lyrics, full screen"))
				save(r, "full-screen-lyrics-"+label)
			}
			e := newNPRigWith(t, opts, func(r *npRig) { r.width, r.height = size[0], size[1] })
			e.lyr.set(services.Lyrics{}, lyrics.ErrNotFound)
			e.play(0)
			e.open("")
			e.tt.Until("empty", e.tt.Shows("No lyrics for this song"))
			save(e, "no-lyrics-"+label)
		}
	}
}

func TestUnderReducedMotionThePageJumpsInsteadOfScrollingSmoothly(t *testing.T) {
	theme.SetReducedMotion(true)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Word", Lines: timedLines(60, true)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	r.settle()
	sh := r.p.np.sheet
	r.play(1000 + 40*4000 + 500)
	// One frame later the sheet is already there: no glide to wait for.
	r.advance(20 * time.Millisecond)
	r.settle()
	if sh.scroll.Y <= 0 || sh.scroll.Scrolling() {
		t.Errorf("scrolled %v, gliding %v: want an instant jump", sh.scroll.Y, sh.scroll.Scrolling())
	}
	// Words light whole, with no sweep between them: the shaped texts have
	// zero-length words.
	for k, s := range sh.sweeps {
		if !k.whole {
			t.Fatalf("a sweep for line %d is shaped for motion", k.line)
		}
		_ = s
	}
}

func TestSavedChoicesShowOnTheFirstOpening(t *testing.T) {
	r := newNPRig(t, func(r *npRig) { r.p.np.prefsInit = false })
	// The settings (as the cache holds them) have the translation on.
	store.Set(r.h.Env.Queries, store.KeySettings, config.Settings{LyricsView: config.LyricsView{Translation: true}})
	lines := timedLines(6, false)
	for i := range lines {
		lines[i].Translation = &services.LyricText{Text: fmt.Sprintf("Translated %d", i)}
	}
	r.lyr.set(services.Lyrics{Timing: "Line", TranslationLanguage: "en-US", Lines: lines}, nil)
	r.play(2000)
	r.open("")
	// The first build with lyrics reads the saved choices, not the defaults.
	r.tt.Until("the saved translation", r.tt.Shows("Translated 1"))
}

func TestEachOpeningStartsAFreshSheet(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(60, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	first := r.p.np.sheet
	first.focused = 7
	r.p.ui.closeOverlay()
	r.settle()
	r.play(2000)
	r.p.ui.openNowPlaying("")
	r.settle()
	if r.p.np.sheet == first || r.p.np.sheet == nil || r.p.np.sheet.focused != -1 {
		t.Errorf("the sheet was kept across openings: %p %p focused %v", first, r.p.np.sheet, r.p.np.sheet)
	}
}

func TestResizingKeepsOneShapedTextPerPart(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Word", Lines: timedLines(6, true)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	r.settle()
	n := len(r.p.np.sheet.sweeps)
	if n == 0 {
		t.Fatal("no sweep was shaped")
	}
	for _, w := range []int{1000, 900, 1100, 960} {
		r.tt.SetSize(w, 700)
		r.settle()
	}
	if got := len(r.p.np.sheet.sweeps); got != n {
		t.Errorf("%d shaped texts after resizing, was %d", got, n)
	}
}

func TestNextSweepEdgeIsTheNextWordLineEndOrRest(t *testing.T) {
	spans := []lyricsync.Span{{BeginMs: 1000, EndMs: 4000}, {BeginMs: 9000, EndMs: 12000}}
	words := []services.LyricWord{{BeginMs: 1000, EndMs: 1500}, {BeginMs: 2000, EndMs: 2500}}
	for _, c := range []struct{ at, want int64 }{
		{0, 1000}, {1200, 1500}, {1500, 2000}, {2500, 4000}, {4000, 4500}, {4500, lyricsync.Never},
	} {
		if got := nextSweepEdge(spans, 0, words, c.at); got != c.want {
			t.Errorf("after %d: %d, want %d", c.at, got, c.want)
		}
	}
}
