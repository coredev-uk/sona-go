package uitest

import (
	"bytes"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Session is a ui.Tester over an Env: what the view posts (service results,
// cover decodes) runs when the test drains it, and Until draws frames until
// the goroutines it waits for have delivered.
type Session struct {
	*ui.Tester
	T testing.TB
	H *Harness
	// Art are the artwork loaders Settle waits for besides the Harness's: a
	// view that builds its own (the app shell does) adds it here.
	Art []*kit.ArtLoader
}

// Tester runs view in a w×h window over this Harness; the view is wrapped
// in theme.Apply as the shell's is.
func (h *Harness) Tester(t testing.TB, view func(*ui.Context), w, hgt int) *Session {
	t.Helper()
	registerFonts(t)
	tt := &Session{T: t, H: h}
	tt.Tester = ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		view(c)
	}, w, hgt)
	return tt
}

// Tester is Of(env).Tester: a view in a w×h window over env (from Env).
func Tester(t testing.TB, env *module.Env, view func(*ui.Context), w, h int) *Session {
	t.Helper()
	hn := Of(env)
	if hn == nil {
		t.Fatal("uitest: Tester needs an Env built by uitest")
	}
	return hn.Tester(t, view, w, h)
}

// HangGuard is how long a wait on state lasts before the test calls it hung.
// It guards against a wake-up that never comes and is not a speed limit: a
// shared runner under -race only makes a wait longer (docs/testing.md,
// "Timing in tests").
const HangGuard = 30 * time.Second

// Until runs what was posted and draws frames until ok holds, waiting for
// the service calls and cover decodes that run on goroutines. It fails the
// test after HangGuard, naming what it waited for and the texts shown.
//
// It draws a frame when the picture can have changed (the first time, when
// something was posted, when a frame is pending) and otherwise at least
// every 50 ms, for state that reaches the view without a post (a toast a
// goroutine raises through MyGo, a value read under a lock). It sleeps in
// MainThread.Wait, which returns the moment a goroutine posts: a frame costs
// milliseconds under -race, and drawing one every 5 ms while a goroutine
// decodes a cover took that goroutine's CPU.
func (tt *Session) Until(what string, ok func() bool) {
	tt.T.Helper()
	const idleFrame = 50 * time.Millisecond
	var last time.Time
	for deadline := time.Now().Add(HangGuard); time.Now().Before(deadline); tt.H.Main.Wait(5 * time.Millisecond) {
		if n := tt.H.Main.Drain(); last.IsZero() || n > 0 || tt.FramePending() || time.Since(last) >= idleFrame {
			tt.Frame()
			last = time.Now()
		}
		if ok() {
			return
		}
	}
	tt.T.Fatalf("timed out waiting for %s; texts %q", what, tt.Texts())
}

// Shows is the condition "every text is on screen", for Until.
func (tt *Session) Shows(texts ...string) func() bool {
	return func() bool {
		for _, w := range texts {
			if !tt.HasText(w) {
				return false
			}
		}
		return true
	}
}

// ShowsExactly is the condition "an element shows each text, as its whole
// text or label", for Until: what MustClick needs to find. Shows (like
// HasText) matches parts of a text, so Shows("Play") holds on the bar's
// "Player" before the Play button is there.
func (tt *Session) ShowsExactly(texts ...string) func() bool {
	return func() bool {
		for _, w := range texts {
			if _, ok := tt.Find(w); !ok {
				return false
			}
		}
		return true
	}
}

// Drain runs what was posted and draws a frame.
func (tt *Session) Drain() {
	tt.H.Main.Drain()
	tt.Frame()
}

// Quiesce is Drain for a test inside a synctest bubble: it waits until every
// goroutine of the bubble (the service calls a click started) has finished or
// blocked for good, then runs what they posted. A test of "nothing happened"
// ends with it instead of a sleep.
func (tt *Session) Quiesce() {
	synctest.Wait()
	tt.Drain()
}

// artPending is how many covers the loaders in play still have in flight.
func (tt *Session) artPending() int {
	n := tt.H.Env.Art.Pending()
	for _, l := range tt.Art {
		n += l.Pending()
	}
	return n
}

// Settle draws frames, running what is posted, until the picture no longer
// changes (a run of quiet frames with nothing queued and no cover still
// being decoded): the covers drawn, the animations done. For a golden image.
//
// It is bounded by what happens, not by how long it takes: a full-window frame
// of the CPU renderer under -race takes most of a second, and several while
// the rest of the suite runs, so a time limit would fail a picture that is
// settling. It fails after settleFrames frames still changing (an animation
// that never ends), or when for HangGuard nothing was posted, drawn
// differently or settled further (a cover or a call that never comes).
//
// It knows only of the covers and the posts: wait (Until) for the page's own
// data before settling, or a golden is taken of the loading state.
func (tt *Session) Settle() {
	tt.T.Helper()
	const quiet = 8
	const settleFrames = 200
	var last []byte
	same := 0
	moved := time.Now() // when something last happened
	for range settleFrames {
		posted := tt.H.Main.Drain()
		tt.Frame()
		tt.Advance(time.Second) // animations and toasts
		img := tt.Image()
		changed := last == nil || !bytes.Equal(last, img.Pix)
		if !changed && !tt.H.Main.Pending() && tt.artPending() == 0 {
			if same++; same >= quiet {
				return
			}
			moved = time.Now()
		} else {
			same = 0
			if changed || posted > 0 {
				moved = time.Now()
			}
		}
		if time.Since(moved) > HangGuard {
			tt.T.Fatalf("the picture did not settle: for %v nothing was posted or drawn differently, with %d covers still in flight", HangGuard, tt.artPending())
		}
		last = append(last[:0], img.Pix...)
		tt.H.Main.Wait(10 * time.Millisecond) // a post from a goroutine ends the wait
	}
	tt.T.Fatalf("the picture was still changing after %d frames", settleFrames)
}

// registerFonts registers the interface font (Schibsted Grotesk; once per
// process) before the first frame a test draws: a golden drawn in the
// system's sans matches only the machine that drew it.
func registerFonts(t testing.TB) {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
}
