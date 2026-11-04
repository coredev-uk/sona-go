package library

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// videoFixtures are n music videos; the first has a catalogue ID, the
// second none (it opens by its library ID), the third is explicit.
func videoFixtures(n int) []services.MusicVideo {
	out := make([]services.MusicVideo, n)
	for i := range out {
		out[i] = services.MusicVideo{
			ID: fmt.Sprintf("v.%d", i), Source: services.SourceLibrary, Name: fmt.Sprintf("Video %d", i),
			Artist: fmt.Sprintf("Band %d", i), DurationMs: 205_000 + int64(i)*60_000,
		}
		if i != 1 {
			out[i].CatalogID = fmt.Sprintf("cat%d", i)
		}
	}
	if n > 2 {
		out[2].Explicit = true
	}
	return out
}

func (s *gridStub) setVideos(v []services.MusicVideo) {
	s.mu.Lock()
	s.videos = v
	s.mu.Unlock()
}

// videoPlays notes the queues started instead of starting them.
type videoPlays struct {
	playbackAPI
	mu    sync.Mutex
	refs  [][]services.TrackRef
	start []int
}

func (p *videoPlays) PlayItems(_ context.Context, refs []services.TrackRef, start int, _ services.PlayFrom) error {
	p.mu.Lock()
	p.refs, p.start = append(p.refs, refs), append(p.start, start)
	p.mu.Unlock()
	return nil
}

func (p *videoPlays) played() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.refs)
}

// newVideosRig is the Music videos page over videos, with an account signed
// in; plays notes what would play when videos can.
func newVideosRig(t *testing.T, videos []services.MusicVideo, canPlay bool, opts ...uitest.Option) (*gridRig, *videoPlays) {
	t.Helper()
	plays := &videoPlays{}
	r := newGridRig(t, videosPath, func(l *library, s *gridStub) {
		fakeAuth(l, gridAuth{"signed_in"})
		s.videos = videos
		plays.playbackAPI = l.play
		l.play = plays
		if canPlay {
			l.env.Players.RegisterVideo(func(services.MusicVideo, *ui.Handle) {})
			l.env.Players.Capabilities = func() *services.Capabilities {
				return &services.Capabilities{Playback: services.PlaybackCapabilities{Video: true}}
			}
		}
	}, opts...)
	return r, plays
}

func TestMusicVideosPageShowsTheVideos(t *testing.T) {
	r, _ := newVideosRig(t, videoFixtures(3), false)
	r.tt.Until("the videos", r.tt.Shows("Music videos", "3 music videos", "Video 0", "Video 1", "Video 2",
		"Band 0 · 3:25", "Band 1 · 4:25", "Band 2 · 5:25"))
	want := []string{"Video 0", "Video 1", "Video 2"}
	if got := r.order(want...); !slices.Equal(got, want) {
		t.Errorf("videos shown %q, want %q", got, want)
	}
	if asks := r.stub.asks(); len(asks) != 1 || asks[0] != "videos offset=0 limit=100" {
		t.Errorf("service asks %q", asks)
	}
	// The grid's tiles are 16:9 and wide: three of them fit a row of 1132.
	r.tt.Until("the grid", func() bool {
		a, ok1 := r.tt.Find("Video 0")
		b, ok2 := r.tt.Find("Video 2")
		return ok1 && ok2 && a.Y == b.Y
	})
}

func TestMusicVideosAreReadAHundredAtATime(t *testing.T) {
	r, _ := newVideosRig(t, videoFixtures(250), false)
	r.tt.Until("every video", r.tt.Shows("250 music videos"))
	want := []string{"videos offset=0 limit=100", "videos offset=100 limit=100", "videos offset=200 limit=100"}
	if asks := r.stub.asks(); !slices.Equal(asks, want) {
		t.Errorf("service asks %q, want %q", asks, want)
	}
}

func TestMusicVideosLoadingShowsNoStateYet(t *testing.T) {
	r := newGridRig(t, videosPath, func(l *library, s *gridStub) {
		fakeAuth(l, gridAuth{"signed_in"})
		s.videos = videoFixtures(2)
		s.hold = make(chan struct{})
	})
	for range 20 {
		r.tt.Drain()
	}
	for _, w := range []string{"Music videos", "No music videos in your library", "Sign in to see your music videos"} {
		if got := r.tt.HasText(w); got != (w == "Music videos") {
			t.Errorf("while loading %q shown: %v", w, got)
		}
	}
	if r.tt.HasText("2 music videos") {
		t.Error("the count shows before the list is read")
	}
	close(r.stub.hold)
	r.tt.Until("the videos", r.tt.Shows("Video 0", "2 music videos"))
}

func TestMusicVideosPlayOnlyWhereVideosPlay(t *testing.T) {
	// The test platform's engine does not play videos: no control, no menu.
	r, _ := newVideosRig(t, videoFixtures(3), false)
	r.tt.Until("the videos", r.tt.Shows("Video 0"))
	if r.l.canPlayVideo() {
		t.Fatal("the test platform reports that videos play")
	}
	if _, ok := r.tt.Find("Play Video 0"); ok {
		t.Error("a tile has a play control where videos do not play")
	}
	if err := r.tt.ClickWith(ui.Cmd, "Video 0"); err != nil {
		t.Fatal(err)
	}
	if n := r.l.videos.cover.Sel.Len(); n != 0 {
		t.Errorf("a tile with no menu target was selected (%d)", n)
	}

	// Where they play, the control starts that video alone.
	p, plays := newVideosRig(t, videoFixtures(3), true)
	p.tt.Until("the videos", p.tt.Shows("Video 0", "Video 2"))
	if err := p.tt.Click("Play Video 1"); err != nil {
		t.Fatal(err)
	}
	p.tt.Until("the queue started", func() bool { return plays.played() > 0 })
	plays.mu.Lock()
	defer plays.mu.Unlock()
	if len(plays.refs) != 1 || plays.start[0] != 0 || len(plays.refs[0]) != 1 {
		t.Fatalf("started %d queues at %v", len(plays.refs), plays.start)
	}
	if got := plays.refs[0][0]; got.ID != "v.1" || got.Kind != services.TrackKindVideo {
		t.Errorf("the queue's item is %+v, want the video v.1", got)
	}
	p.tt.Until("the video view opened", p.h.Env.Video.IsOpen)
	if got := p.router.Location(); got != videosPath {
		t.Errorf("playing navigated to %q (the video view is the video page's)", got)
	}
	// With a target a tile can be selected.
	if err := p.tt.ClickWith(ui.Cmd, "Video 2"); err != nil {
		t.Fatal(err)
	}
	if n := p.l.videos.cover.Sel.Len(); n != 1 {
		t.Errorf("Ctrl+click selected %d tiles", n)
	}
}

// Closing the video view gives the focus back to the tile's Play control that
// opened it.
func TestClosingTheVideoViewReturnsTheFocusToTheTilesPlay(t *testing.T) {
	r, plays := newVideosRig(t, videoFixtures(3), true)
	r.tt.Until("the videos", r.tt.Shows("Video 0", "Video 2"))
	if err := r.tt.Click("Play Video 1"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the queue started", func() bool { return plays.played() > 0 })
	r.tt.Until("the video view opened", r.h.Env.Video.IsOpen)
	r.tt.Key(0, ui.KeyTab) // the focus moves into the view
	if r.tt.Focused("Play Video 1") {
		t.Fatal("the focus did not leave the tile's Play")
	}
	r.h.Env.Video.Close()
	r.tt.Until("the focus back on the tile's Play", func() bool { return r.tt.Focused("Play Video 1") })
}

func TestMusicVideoTilesOpenTheirPage(t *testing.T) {
	r, _ := newVideosRig(t, videoFixtures(3), false)
	r.tt.Until("the videos", r.tt.Shows("Video 0", "Video 1"))
	if err := r.tt.Click("Video 0"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/music-video/cat0" {
		t.Errorf("Video 0 opened %q", got)
	}
	r.router.Replace(videosPath)
	r.tt.Until("the page again", r.tt.Shows("Video 1"))
	if err := r.tt.Click("Video 1"); err != nil {
		t.Fatal(err)
	}
	// A video with no catalogue ID opens by its library ID.
	if got := r.router.Location(); got != "/music-video/v.1" {
		t.Errorf("Video 1 opened %q", got)
	}

	bare := newGridRig(t, videosPath, func(l *library, s *gridStub) {
		fakeAuth(l, gridAuth{"signed_in"})
		s.videos = videoFixtures(2)
		l.env.HasRoute = func(string) bool { return false }
	})
	bare.tt.Until("the videos", bare.tt.Shows("Video 0"))
	_ = bare.tt.Click("Video 0")
	if got := bare.router.Location(); got != videosPath {
		t.Errorf("a tile with no page navigated to %q", got)
	}
}

func TestMusicVideosKeyboardAndSelection(t *testing.T) {
	r, _ := newVideosRig(t, videoFixtures(9), true)
	r.tt.Until("the videos", r.tt.Shows("Video 0", "Video 8"))
	sel := &r.l.videos.cover.Sel
	if err := r.tt.ClickWith(ui.Cmd, "Video 1"); err != nil {
		t.Fatal(err)
	}
	if sel.Len() != 1 || r.router.Location() != videosPath {
		t.Fatalf("Ctrl+click: %d selected, location %q", sel.Len(), r.router.Location())
	}
	r.tt.Key(0, ui.KeyRight)
	if !r.tt.Focused("Video 2") {
		t.Errorf("Right did not move to the next tile")
	}
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Key(0, ui.KeyLeft)
	// Down moves a row: as many tiles on as share the first one's line.
	first, _ := r.tt.Find("Video 0")
	cols := 0
	for i := range 9 {
		if tile, ok := r.tt.Find(fmt.Sprintf("Video %d", i)); ok && tile.Y == first.Y {
			cols++
		}
	}
	if cols < 3 {
		t.Fatalf("%d tiles to a row", cols)
	}
	r.tt.Key(0, ui.KeyDown)
	if want := fmt.Sprintf("Video %d", cols); !r.tt.Focused(want) {
		t.Errorf("Down did not move to %s (%d columns)", want, cols)
	}
	r.tt.Key(0, ui.KeyUp)
	if !r.tt.Focused("Video 0") {
		t.Errorf("Up did not move back a row")
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(ui.Shift, ui.KeyRight)
	r.tt.Key(ui.Shift, ui.KeyRight)
	if sel.Len() != 3 {
		t.Errorf("Shift+Right twice selected %d tiles", sel.Len())
	}
	r.tt.Key(0, ui.KeyEscape)
	if err := r.tt.ClickWith(ui.Cmd, "Video 0"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.ClickWith(ui.Shift, "Video 2"); err != nil {
		t.Fatal(err)
	}
	if sel.Len() != 3 || r.router.Location() != videosPath {
		t.Errorf("Shift+click: %d selected, location %q", sel.Len(), r.router.Location())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(0, ui.KeyEnter)
	if got := r.router.Location(); !strings.HasPrefix(got, "/music-video/") {
		t.Errorf("Enter opened %q", got)
	}
}

func TestMusicVideosGateStates(t *testing.T) {
	t.Run("signed out", func(t *testing.T) {
		var opened bool
		r := newGridRig(t, videosPath, func(l *library, _ *gridStub) {
			l.openAccount = func() { opened = true }
		}, uitest.Empty())
		r.tt.Until("the sign-in text", r.tt.Shows("Sign in to see your music videos",
			"Music videos you add in Apple Music are listed here after you sign in.", "Music videos"))
		r.settle()
		if err := r.tt.Click("Sign in"); err != nil {
			t.Fatal(err)
		}
		r.tt.Drain()
		if !opened {
			t.Error("Sign in did not open the account dialog")
		}
	})
	t.Run("empty", func(t *testing.T) {
		r, _ := newVideosRig(t, []services.MusicVideo{}, false)
		r.tt.Until("the empty text", r.tt.Shows("No music videos in your library",
			"Music videos you add in Apple Music on any device are listed here."))
		if r.tt.HasText("0 music videos") {
			t.Error("an empty list counts its videos")
		}
	})
	t.Run("error", func(t *testing.T) {
		// The library's call needs Apple, and the test network has none.
		r, _ := newVideosRig(t, nil, false)
		r.tt.Until("the error", r.tt.Shows("Couldn't load your music videos from Apple Music", "Try again"))
		r.stub.setVideos(videoFixtures(2))
		if err := r.tt.Click("Try again"); err != nil {
			t.Fatal(err)
		}
		r.tt.Until("the videos", r.tt.Shows("Video 0", "2 music videos"))
	})
}

func TestASyncDoneRefreshesTheMusicVideos(t *testing.T) {
	r, _ := newVideosRig(t, videoFixtures(2), false)
	r.tt.Until("the videos", r.tt.Shows("2 music videos"))
	r.stub.setVideos(videoFixtures(3))
	// Music videos are no sync domain: a finished domain leaves them, the
	// whole sync refreshes everything under "library".
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 1, Total: 1, Finished: true})
	r.tt.Drain()
	if r.tt.HasText("3 music videos") {
		t.Fatal("a finished domain refetched the music videos")
	}
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true})
	r.tt.Until("the new video", r.tt.Shows("3 music videos"))
}
