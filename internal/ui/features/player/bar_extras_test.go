package player

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// navRecorder is the router the bar pushes pages to.
type navRecorder struct{ pushed []string }

func (n *navRecorder) Push(path string)    { n.pushed = append(n.pushed, path) }
func (n *navRecorder) Replace(path string) {}
func (n *navRecorder) Back()               {}
func (n *navRecorder) Forward()            {}
func (n *navRecorder) Path() string        { return "/" }
func (n *navRecorder) Location() string    { return "/" }

// routed makes every page a registered one, or none.
func routed(r *rig, all bool) *navRecorder {
	nav := &navRecorder{}
	r.env.Nav = nav
	r.env.HasRoute = func(string) bool { return all }
	return nav
}

func TestTitleAndArtistLinkToPagesThatExist(t *testing.T) {
	r := newRig(t, true)
	nav := routed(r, true)
	r.seed(func(s *services.PlaybackState) {
		s.Current.AlbumID, s.Current.ArtistID = "al.9", "ar.7"
	})
	if err := r.tt.Click("Song Title"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("The Artist"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/album/al.9?song=i.1", "/artist/ar.7"}; !slices.Equal(nav.pushed, want) {
		t.Errorf("pushed %q, want %q", nav.pushed, want)
	}
	if !r.tt.HasText("The Album") {
		t.Errorf("the album is missing: %q", r.tt.Texts())
	}
}

func TestLinksArePlainTextWithoutTheirPage(t *testing.T) {
	r := newRig(t, true)
	nav := routed(r, false)
	r.seed(func(s *services.PlaybackState) {
		s.Current.AlbumID, s.Current.ArtistID = "al.9", "ar.7"
	})
	_ = r.tt.Click("Song Title")
	if len(nav.pushed) != 0 {
		t.Errorf("pushed %q for a page that is not ported", nav.pushed)
	}
	if !r.tt.HasText("The Artist · The Album") {
		t.Errorf("the line is not whole: %q", r.tt.Texts())
	}
}

func TestTitleLinksOfStationsAndVideos(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*services.QueueItem)
		want string
	}{
		{"a station", func(q *services.QueueItem) { q.Live = true }, "/station/i.1"},
		{"a music video", func(q *services.QueueItem) { q.Video, q.CatalogID = true, "mv.5" }, "/music-video/mv.5"},
		{"a broadcaster", func(q *services.QueueItem) { q.Live, q.Broadcast = true, true }, ""},
	} {
		r := newRig(t, true)
		nav := routed(r, true)
		r.seed(func(s *services.PlaybackState) { tc.mut(s.Current) })
		_ = r.tt.Click("Song Title")
		got := ""
		if len(nav.pushed) > 0 {
			got = nav.pushed[0]
		}
		if got != tc.want {
			t.Errorf("%s: pushed %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCoverButtonOpensAndClosesNowPlaying(t *testing.T) {
	r := newRig(t, true)
	r.tt.Frame()
	if _, ok := r.tt.Find("Nothing playing"); !ok {
		t.Fatalf("no disabled cover button while idle: %q", r.tt.Texts())
	}
	_ = r.tt.Click("Nothing playing")
	if r.p.ui.overlay != overlayNone {
		t.Error("an idle cover opened now playing")
	}
	r.seed()
	if err := r.tt.Click("Open now playing"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if r.p.ui.overlay != overlayNowPlaying {
		t.Fatalf("overlay %v", r.p.ui.overlay)
	}
	if err := r.tt.Click("Close now playing"); err != nil {
		t.Fatalf("the button should now close it: %v", err)
	}
	if r.p.ui.overlay != overlayNone {
		t.Errorf("overlay %v after closing", r.p.ui.overlay)
	}
}

func TestQueueToggleShowsAndHidesTheQueue(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	if r.p.ui.queueOpen {
		t.Fatal("the queue starts open")
	}
	if err := r.tt.Click("Queue"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if !r.p.ui.queueOpen {
		t.Error("the Queue button did not show the queue")
	}
	_ = r.tt.Click("Queue")
	if r.p.ui.queueOpen {
		t.Error("the Queue button did not hide the queue")
	}
}

func TestLyricsButtonFollowsTheLyricsSurface(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	_, shown := r.tt.Find("Lyrics")
	if shown != r.p.lyricsSurface() {
		t.Errorf("Lyrics button shown %v, lyricsSurface %v", shown, r.p.lyricsSurface())
	}
}

// fakeRatings is the library service behind the rating buttons.
// wait blocks until a call was made (the test fails after 5 s).
func (f *fakeRatings) wait(t *testing.T) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the rating call never came")
	}
}

type fakeRatings struct {
	mu     sync.Mutex
	state  services.ItemState
	calls  []string
	failed error
	done   chan struct{}
}

func (f *fakeRatings) States(context.Context, []services.ItemRef) ([]services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return []services.ItemState{f.state}, nil
}

func (f *fakeRatings) record(call string, on bool, value int) (services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.state
	f.calls = append(f.calls, fmt.Sprint(call, " ", on))
	if f.failed == nil {
		st.Rating = 0
		if on {
			st.Rating = value
		}
		f.state = st
	}
	f.done <- struct{}{}
	return st, f.failed
}

func (f *fakeRatings) Remove(context.Context, services.ItemRef) (services.ItemState, error) {
	return services.ItemState{}, nil
}

func (f *fakeRatings) Add(context.Context, string, []string) error { return nil }

func (f *fakeRatings) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.record("favorite "+r.ID, on, 1)
}

func (f *fakeRatings) SuggestLess(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.record("suggest-less "+r.ID, on, -1)
}

func ratedRig(t *testing.T, state services.ItemState) (*rig, *fakeRatings) {
	t.Helper()
	r := newRig(t, true)
	f := &fakeRatings{state: state, done: make(chan struct{}, 8)}
	r.env.ItemStates = kit.NewItemStates(r.env.Queries, r.env.Post, f)
	r.seed(func(s *services.PlaybackState) { s.Current.CatalogID = "100" })
	// The buttons are disabled until Go has answered the (batched) state load.
	for range 400 {
		r.h.Main.Drain()
		r.tt.Frame()
		if r.env.ItemStates.Use(kit.StateSongs, kit.StateItem{ID: "i.1", CatalogID: "100"}).HasData {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return r, f
}

// until runs the main thread's posted work and frames until the element
// shows.
func (r *rig) until(t *testing.T, name string) {
	t.Helper()
	for range 200 {
		r.h.Main.Drain()
		r.tt.Frame()
		if _, ok := r.tt.Find(name); ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%q never showed: %q", name, r.tt.Texts())
}

func TestRatingButtonsFavouriteTheSong(t *testing.T) {
	r, f := ratedRig(t, services.ItemState{Kind: "songs", ID: "i.1"})
	r.until(t, "Favourite Song Title")
	if _, ok := r.tt.Find("Suggest less like Song Title"); ok {
		t.Error("the bar's compact buttons show Suggest Less")
	}
	if err := r.tt.Click("Favourite Song Title"); err != nil {
		t.Fatal(err)
	}
	f.wait(t)
	r.until(t, "Undo favourite for Song Title")
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.calls, []string{"favorite i.1 true"}) {
		t.Errorf("calls %q", f.calls)
	}
}

func TestRatingButtonsUndoAFavourite(t *testing.T) {
	r, f := ratedRig(t, services.ItemState{Kind: "songs", ID: "i.1", Rating: 1})
	r.until(t, "Undo favourite for Song Title")
	_ = r.tt.Click("Undo favourite for Song Title")
	f.wait(t)
	r.until(t, "Favourite Song Title")
}

// inBubble runs a test body in a synctest bubble on one core: MyGo's rasteriser
// shares helper goroutines between frames, and a helper (or the WaitGroup it
// signals) that belongs to a bubble breaks the next frame drawn outside it. One
// core draws without helpers.
func inBubble(t *testing.T, body func(*testing.T)) {
	t.Helper()
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	synctest.Test(t, body)
}

func TestNoRatingButtonsForLiveVideoOrUnknownSongs(t *testing.T) {
	for name, mut := range map[string]func(*services.QueueItem){
		"live":  func(q *services.QueueItem) { q.CatalogID, q.Live = "100", true },
		"video": func(q *services.QueueItem) { q.CatalogID, q.Video = "100", true },
		"none":  func(q *services.QueueItem) {},
	} {
		t.Run(name, func(t *testing.T) {
			inBubble(t, func(t *testing.T) {
				r := newRig(t, true)
				r.env.ItemStates = kit.NewItemStates(r.env.Queries, r.env.Post, &fakeRatings{done: make(chan struct{}, 8)})
				r.seed(func(s *services.PlaybackState) { mut(s.Current) })
				r.tt.Quiesce() // the state load, if one was started, has answered
				if _, ok := r.tt.Find("Favourite Song Title"); ok {
					t.Errorf("%s: Favourite shown", name)
				}
			})
		})
	}
}

func TestFavouriteShortcutTogglesTheRating(t *testing.T) {
	r, f := ratedRig(t, services.ItemState{Kind: "songs", ID: "i.1", Rating: 1})
	r.p.toggleFavourite()
	// The state arrives on a goroutine and the change is posted to the main thread.
	for done := false; !done; {
		r.h.Main.Drain()
		select {
		case <-f.done:
			done = true
		case <-time.After(5 * time.Millisecond):
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !slices.Equal(f.calls, []string{"favorite i.1 false"}) {
		t.Errorf("a favourite should be undone: %q", f.calls)
	}
}

func TestFavouriteShortcutIgnoresWhatCannotBeRated(t *testing.T) {
	inBubble(t, favouriteShortcutIgnoresWhatCannotBeRated)
}

func favouriteShortcutIgnoresWhatCannotBeRated(t *testing.T) {
	r, f := ratedRig(t, services.ItemState{})
	r.seed(func(s *services.PlaybackState) { s.Current.Live = true })
	r.p.toggleFavourite()
	r.tt.Quiesce() // a call that got through has run by now
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) != 0 {
		t.Errorf("calls %q for live radio", f.calls)
	}
}

func TestFavouriteAndQueueShortcutsExist(t *testing.T) {
	r := newRig(t, true)
	got := map[string]string{}
	for _, s := range r.p.shortcuts() {
		got[s.ID] = fmt.Sprint(s.Group, s.Keys)
	}
	if got["player.favourite"] != "playback[F]" || got["player.queue"] != "view[Q]" {
		t.Errorf("shortcuts %v", got)
	}
}

func TestSignInAgainOpensTheAccountDialog(t *testing.T) {
	r := newRig(t, true)
	r.env.Surfaces = store.NewSurfaces()
	r.seed(func(s *services.PlaybackState) { s.AuthRequired = true })
	if err := r.tt.Click("Sign in again to keep playing"); err != nil {
		t.Fatal(err)
	}
	if !r.env.Surfaces.AccountOpen() {
		t.Error("the sign-in line did not open the account dialog")
	}
}

func TestVideoItemShowsTheVideoButtonInsteadOfLyricsAndTheCoverOpensIt(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Current.Video = true })
	if _, ok := r.tt.Find("Lyrics"); ok {
		t.Error("a music video still shows Lyrics")
	}
	if _, ok := r.tt.Find("Show video"); !ok {
		t.Fatalf("no Show video: %q", r.tt.Texts())
	}
	if err := r.tt.Click("Show video"); err != nil {
		t.Fatal(err)
	}
	if !r.env.Video.IsOpen() {
		t.Error("Show video did not open the video")
	}
	r.env.Video.Close()
	r.tt.Frame()
	if err := r.tt.Click("Show video"); err != nil {
		t.Fatal(err)
	}
}

func TestANewVideoItemOpensTheVideoView(t *testing.T) {
	r := newRig(t, true)
	r.p.init()
	st := store.EmptyState()
	st.State = store.StatePlaying
	st.Current = &services.QueueItem{UID: 7, ID: "mv.1", Name: "Clip", Video: true}
	r.env.Bus.Emit(services.EventPlaybackState, st)
	r.h.Main.Drain()
	if !r.env.Video.IsOpen() {
		t.Fatal("a new video item did not open the view")
	}
	r.env.Video.Close()
	r.env.Bus.Emit(services.EventPlaybackState, st)
	r.h.Main.Drain()
	if r.env.Video.IsOpen() {
		t.Error("the same item reopened the view")
	}
}

// A video that is paused (a restored queue's first state) or failed does not
// pop the view open; the next one that starts loading does.
func TestAPausedOrRestoredVideoDoesNotOpenTheVideoView(t *testing.T) {
	r := newRig(t, true)
	r.p.init()
	for _, state := range []string{store.StatePaused, store.StateError} {
		st := store.EmptyState()
		st.State = state
		st.Current = &services.QueueItem{UID: 7, ID: "mv.1", Name: "Clip", Video: true}
		r.env.Bus.Emit(services.EventPlaybackState, st)
		r.h.Main.Drain()
		if r.env.Video.IsOpen() {
			t.Fatalf("a %v video opened the view", state)
		}
	}
	st := store.EmptyState()
	st.State = store.StateLoading
	st.Current = &services.QueueItem{UID: 8, ID: "mv.2", Name: "Clip 2", Video: true}
	r.env.Bus.Emit(services.EventPlaybackState, st)
	r.h.Main.Drain()
	if !r.env.Video.IsOpen() {
		t.Error("a loading video did not open the view")
	}
}
