package uitest

import (
	"slices"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Nav is a module.Navigator that records the pages pushed to it.
type Nav struct {
	Pushed   []string
	Replaced []string
}

func (n *Nav) Push(path string)    { n.Pushed = append(n.Pushed, path) }
func (n *Nav) Replace(path string) { n.Replaced = append(n.Replaced, path) }
func (n *Nav) Back()               {}
func (n *Nav) Forward()            {}
func (n *Nav) Path() string        { return "/" }
func (n *Nav) Location() string    { return "/" }

// Went reports whether path was pushed.
func (n *Nav) Went(path string) bool { return slices.Contains(n.Pushed, path) }

// Last is the page pushed last, "" for none.
func (n *Nav) Last() string {
	if len(n.Pushed) == 0 {
		return ""
	}
	return n.Pushed[len(n.Pushed)-1]
}

// Play seeds the playback mirror with a playing catalogue song, "Song Title"
// by "The Artist" on "The Album", 4 minutes long, at posMs, with every
// control. mutate changes the state before it is seeded. It returns the
// state seeded.
func (h *Harness) Play(posMs int64, mutate ...func(*services.PlaybackState)) services.PlaybackState {
	s := store.EmptyState()
	s.State = store.StatePlaying
	s.Current = &services.QueueItem{
		ID: "i.1", CatalogID: "1001", Name: "Song Title", Artist: "The Artist", Album: "The Album",
		ArtistID: "a1", AlbumID: "al1",
	}
	s.DurationMs, s.PositionMs, s.At = 240_000, posMs, h.Clock.Now().UnixMilli()
	s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true}
	for _, m := range mutate {
		m(&s)
	}
	h.Env.Play.Seed(s, services.QueueState{Index: -1})
	return s
}

// Pump advances the clock by step and draws a frame, n times: for what a
// click or a key sets going (a menu closing, a transition).
func (tt *Session) Pump(n int, step time.Duration) {
	for range n {
		tt.Advance(step)
		tt.Drain()
	}
}

// MustClick clicks the element labelled label and fails the test, with the
// texts shown, if there is none.
func (tt *Session) MustClick(label string) {
	tt.T.Helper()
	if err := tt.Click(label); err != nil {
		tt.T.Fatalf("click %q: %v; texts %q", label, err, tt.Texts())
	}
}

// Want fails the test for each of texts that is not shown.
func (tt *Session) Want(texts ...string) {
	tt.T.Helper()
	for _, s := range texts {
		if !tt.HasText(s) {
			tt.T.Errorf("%q is not shown; texts %q", s, tt.Texts())
		}
	}
}

// Lacks fails the test for each of texts that is shown.
func (tt *Session) Lacks(texts ...string) {
	tt.T.Helper()
	for _, s := range texts {
		if tt.HasText(s) {
			tt.T.Errorf("%q is shown; texts %q", s, tt.Texts())
		}
	}
}

// Emit sends Go's event to the UI and draws what it changed.
func (tt *Session) Emit(event string, payload ...any) {
	tt.H.Bus.Emit(event, payload...)
	tt.Drain()
}

// Hover rests the pointer on the element named name until its tip shows.
func (tt *Session) Hover(name string) {
	tt.T.Helper()
	n, ok := tt.Find(name)
	if !ok {
		tt.T.Fatalf("no %q to hover; texts %q", name, tt.Texts())
	}
	tt.Move(n.X+n.W/2, n.Y+n.H/2)
	tt.Advance(time.Second)
}
