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

// Until runs what was posted and draws frames until ok holds, waiting for
// the service calls and cover decodes that run on goroutines. It fails the
// test after 10 s, naming what it waited for and the texts shown.
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
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); tt.H.Main.Wait(5 * time.Millisecond) {
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
func (tt *Session) Settle() {
	tt.T.Helper()
	const quiet = 8
	var last []byte
	same := 0
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		tt.H.Main.Drain()
		tt.Frame()
		tt.Advance(time.Second) // animations and toasts
		img := tt.Image()
		if last != nil && bytes.Equal(last, img.Pix) && !tt.H.Main.Pending() && tt.artPending() == 0 {
			if same++; same >= quiet {
				return
			}
		} else {
			same = 0
		}
		last = append(last[:0], img.Pix...)
	}
	tt.T.Fatal("the picture never settled")
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
