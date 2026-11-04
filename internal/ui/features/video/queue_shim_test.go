package video

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeQueueAPI is PlaybackService's PlayItems and UndoReplace.
type fakeQueueAPI struct {
	mu    sync.Mutex
	refs  [][]services.TrackRef
	start []int
	from  []services.PlayFrom
	err   error
	undid chan struct{}
}

func (f *fakeQueueAPI) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refs, f.start, f.from = append(f.refs, refs), append(f.start, start), append(f.from, from)
	return f.err
}

func (f *fakeQueueAPI) UndoReplace(context.Context) (bool, error) {
	f.undid <- struct{}{}
	return true, nil
}

func (f *fakeQueueAPI) played() int { f.mu.Lock(); defer f.mu.Unlock(); return len(f.refs) }

type shimRig struct {
	q   *queueCommands
	api *fakeQueueAPI
	h   *uitest.Harness
	// mode is what the queue setting says.
	mode string
}

func newShim(t *testing.T) *shimRig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	r := &shimRig{api: &fakeQueueAPI{undid: make(chan struct{}, 1)}, h: h, mode: config.ReplaceUndo}
	r.q = &queueCommands{svc: r.api, play: h.Env.Play, post: h.Env.Post, mode: func() string { return r.mode }}
	return r
}

// settle runs what the goroutines posted until n plays were made.
func (r *shimRig) settle(t *testing.T, plays int) {
	t.Helper()
	r.until(t, "the plays", func() bool { return r.api.played() >= plays })
}

// until drains what the goroutines post until ok holds: a service call's
// result comes back through the poster after the call returns, so one drain
// after the call is not enough.
func (r *shimRig) until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		r.h.Main.Drain()
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func twoVideos() []services.MusicVideo {
	return []services.MusicVideo{
		{ID: "a", CatalogID: "ca", Source: "library", Name: "A"},
		{ID: "b", Source: "catalog", Name: "B"},
	}
}

func TestPlayVideosQueuesVideoRefsWithNoCollectionThenOpens(t *testing.T) {
	r := newShim(t)
	opened := 0
	r.q.PlayVideos(twoVideos(), 1, func() { opened++ })
	r.until(t, "the view to open", func() bool { return opened > 0 })
	want := []services.TrackRef{
		{ID: "a", Source: "library", Kind: services.TrackKindVideo},
		{ID: "b", Source: "catalog", Kind: services.TrackKindVideo},
	}
	if !slices.Equal(r.api.refs[0], want) || r.api.start[0] != 1 || r.api.from[0] != (services.PlayFrom{}) {
		t.Errorf("played %v from %d as %v, want %v from 1 with no collection", r.api.refs, r.api.start, r.api.from, want)
	}
	if opened != 1 {
		t.Errorf("the view opened %d times", opened)
	}
}

func TestPlayVideosFailureToastsAndLeavesTheViewClosed(t *testing.T) {
	r := newShim(t)
	r.api.err = services.ErrNotReleased
	opened := false
	r.q.PlayVideos(twoVideos(), 0, func() { opened = true })
	r.settle(t, 1)
	// The failure's toast comes back through the poster: show it.
	tt0 := r.h.Tester(t, func(c *ui.Context) { kit.OverlayLayer(c) }, 600, 300)
	tt0.Until("the toast", tt0.Shows("Couldn't start playback"))
	if opened {
		t.Error("the view opened for a play that failed")
	}
}

func TestPlayVideosWithNothingDoesNothing(t *testing.T) {
	r := newShim(t)
	r.q.PlayVideos(nil, 0, func() { t.Error("opened") })
	time.Sleep(10 * time.Millisecond)
	if r.api.played() != 0 {
		t.Error("played nothing")
	}
}

// longQueue is a queue with n songs added by hand after the current one.
func longQueue(n int) services.QueueState {
	q := services.QueueState{Index: 0, Items: []services.QueueItem{{UID: 1}}}
	for i := range n {
		q.Items = append(q.Items, services.QueueItem{UID: uint64(10 + i), Added: true})
	}
	return q
}

func TestReplacingALongUserQueueOffersUndoByDefault(t *testing.T) {
	r := newShim(t)
	r.h.Env.Play.Seed(store.EmptyState(), longQueue(4))
	done := false
	r.q.PlayVideos(twoVideos(), 0, func() { done = true })
	r.until(t, "the play to finish", func() bool { return done })
	tt := r.h.Tester(t, func(c *ui.Context) { kit.OverlayLayer(c) }, 600, 300)
	tt.Until("the offer", tt.Shows("Queue replaced", "4 songs you added were removed.", "Undo"))
	if err := tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.api.undid:
	case <-time.After(5 * time.Second):
		t.Fatal("Undo did not ask the service")
	}
}

func TestReplacingAShortUserQueueSaysNothing(t *testing.T) {
	r := newShim(t)
	r.h.Env.Play.Seed(store.EmptyState(), longQueue(3))
	done := false
	r.q.PlayVideos(twoVideos(), 0, func() { done = true })
	r.until(t, "the play to finish", func() bool { return done })
	tt := r.h.Tester(t, func(c *ui.Context) { kit.OverlayLayer(c) }, 600, 300)
	tt.Drain()
	if tt.HasText("Queue replaced") {
		t.Error("an offer for a queue of three")
	}
}

func TestReplaceConfirmAsksFirst(t *testing.T) {
	r := newShim(t)
	r.mode = config.ReplaceConfirm
	r.h.Env.Play.Seed(store.EmptyState(), longQueue(5))
	opened := false
	r.q.PlayVideos(twoVideos(), 0, func() { opened = true })
	tt := r.h.Tester(t, func(c *ui.Context) { kit.OverlayLayer(c) }, 800, 400)
	tt.Until("the question", tt.Shows("Replace your queue?", "The 5 songs you added to the queue will be removed.", "Replace queue"))
	if r.api.played() != 0 {
		t.Fatal("played before the answer")
	}
	if err := tt.Click("Replace queue"); err != nil {
		t.Fatal(err)
	}
	r.until(t, "the view to open after the answer", func() bool { return opened })

	// Cancelling plays nothing.
	r2 := newShim(t)
	r2.mode = config.ReplaceConfirm
	r2.h.Env.Play.Seed(store.EmptyState(), longQueue(5))
	r2.q.PlayVideos(twoVideos(), 0, func() { t.Error("opened after Cancel") })
	tt2 := r2.h.Tester(t, func(c *ui.Context) { kit.OverlayLayer(c) }, 800, 400)
	tt2.Until("the question", tt2.Shows("Replace your queue?"))
	if err := tt2.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if r2.api.played() != 0 {
		t.Error("played after Cancel")
	}
}

func TestPlaySongsDropsTheUnplayableAndToastsWhenNoneCan(t *testing.T) {
	r := newShim(t)
	songs := []services.Song{{ID: "x", Source: "catalog"}, {ID: "y", Source: "catalog", Playable: true}, {ID: "z", Source: "catalog", Playable: true}}
	r.q.PlaySongs(songs, 2, services.PlayFrom{ID: "z"})
	r.settle(t, 1)
	if len(r.api.refs[0]) != 2 || r.api.start[0] != 1 || r.api.from[0].ID != "z" {
		t.Errorf("played %v from %d as %v", r.api.refs, r.api.start, r.api.from)
	}
	r.q.PlaySongs(songs[:1], 0, services.PlayFrom{})
	tt := r.h.Tester(t, func(c *ui.Context) { kit.OverlayLayer(c) }, 600, 300)
	tt.Until("the toast", tt.Shows("Nothing here can be played", "None of these songs is available to stream."))
	if r.api.played() != 1 {
		t.Error("played songs none of which can")
	}
}
