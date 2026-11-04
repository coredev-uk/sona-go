package library

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// gridStub is the library service the grid pages' tests wrap: it notes the
// lists asked for, fails them on demand, and answers the music videos from a
// list (the real call needs Apple).
type gridStub struct {
	libraryAPI
	mu      sync.Mutex
	failing bool
	asked   []string
	videos  []services.MusicVideo
	// hold, when set, keeps MusicVideos waiting until it is closed.
	hold chan struct{}
}

// settle draws frames until the page's layout has stopped moving (an empty
// state's cover slots grow once the page is measured): a click lands where
// the element is found.
func (r *gridRig) settle() {
	r.tt.T.Helper()
	for range 5 {
		r.tt.Drain()
	}
}

func (s *gridStub) fail(on bool) {
	s.mu.Lock()
	s.failing = on
	s.mu.Unlock()
}

// note records a call and reports whether it must fail.
func (s *gridStub) note(call string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.asked = append(s.asked, call)
	if s.failing {
		return errors.New("the library is unreadable")
	}
	return nil
}

func (s *gridStub) asks() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asked)
}

func (s *gridStub) Playlists(ctx context.Context, sort string, desc bool, off, limit int) (services.PlaylistPage, error) {
	if err := s.note(fmt.Sprintf("playlists %s desc=%v offset=%d", sort, desc, off)); err != nil {
		return services.PlaylistPage{}, err
	}
	return s.libraryAPI.Playlists(ctx, sort, desc, off, limit)
}

func (s *gridStub) RecentlyAdded(ctx context.Context, off, limit int) (services.LibraryEntryPage, error) {
	if err := s.note(fmt.Sprintf("recent offset=%d", off)); err != nil {
		return services.LibraryEntryPage{}, err
	}
	return s.libraryAPI.RecentlyAdded(ctx, off, limit)
}

func (s *gridStub) MusicVideos(ctx context.Context, off, limit int) (services.MusicVideoPage, error) {
	if err := s.note(fmt.Sprintf("videos offset=%d limit=%d", off, limit)); err != nil {
		return services.MusicVideoPage{}, err
	}
	if s.hold != nil {
		select {
		case <-s.hold:
		case <-ctx.Done():
			return services.MusicVideoPage{}, ctx.Err()
		}
	}
	s.mu.Lock()
	videos := s.videos
	s.mu.Unlock()
	if videos == nil {
		return s.libraryAPI.MusicVideos(ctx, off, limit)
	}
	page := services.MusicVideoPage{Total: len(videos)}
	if off < len(videos) {
		page.Items = videos[off:min(off+limit, len(videos))]
	}
	return page, nil
}

// gridAuth is an account in the state it names.
type gridAuth struct{ state string }

func (a gridAuth) Status() services.AuthStatus { return services.AuthStatus{State: a.state} }

// gridRig is a page rig with the grid stub in front of the library service;
// the pages whose routes exist (playlist, album, music video) open.
type gridRig struct {
	*pageRig
	stub *gridStub
}

func newGridRig(t *testing.T, start string, tweak func(l *library, s *gridStub), opts ...uitest.Option) *gridRig {
	t.Helper()
	return newGridRigH(t, start, 900, tweak, opts...)
}

// newGridRigH is newGridRig in a window height tall.
func newGridRigH(t *testing.T, start string, height int, tweak func(l *library, s *gridStub), opts ...uitest.Option) *gridRig {
	t.Helper()
	stub := &gridStub{}
	r := newPageRig(t, start, 1132, height, func(l *library) {
		stub.libraryAPI = l.lib
		l.lib = stub
		l.env.HasRoute = func(p string) bool {
			return strings.HasPrefix(p, "/playlist/") || strings.HasPrefix(p, "/album/") || strings.HasPrefix(p, "/music-video/")
		}
		if tweak != nil {
			tweak(l, stub)
		}
	}, opts...)
	return &gridRig{pageRig: r, stub: stub}
}

// order is which of names the page shows, in the order it shows them.
func (r *gridRig) order(names ...string) []string {
	var got []string
	for _, w := range r.tt.Texts() {
		if slices.Contains(names, w) && (len(got) == 0 || got[len(got)-1] != w) {
			got = append(got, w)
		}
	}
	return got
}

// playlistNames are the fixture's playlists, newest added first.
var playlistNames = []string{"Sunday morning", "Focus", "Road trip"}

func TestPlaylistsPageShowsThePlaylists(t *testing.T) {
	r := newGridRig(t, playlistsPath, nil)
	r.tt.Until("the playlists", r.tt.Shows("Playlists", "3 playlists", "Sunday morning", "Focus", "Road trip", "Sort"))
	// "Recently added" is newest first; a playlist with no curator says how
	// many songs it holds.
	if got := r.order(playlistNames...); !slices.Equal(got, playlistNames) {
		t.Errorf("playlists shown %q, want %q", got, playlistNames)
	}
	for _, w := range []string{"5 songs", "4 songs", "6 songs"} {
		if !r.tt.HasText(w) {
			t.Errorf("page lacks the subtitle %q", w)
		}
	}
	if asks := r.stub.asks(); len(asks) == 0 || asks[0] != "playlists dateAdded desc=true offset=0" {
		t.Errorf("first query %q", asks)
	}
	// Library playlists play from their page: no play control on the tile.
	if _, ok := r.tt.Find("Play Focus"); ok {
		t.Error("a playlist tile has a play control")
	}
	r.tt.Until("a cover drawn", func() bool { return r.h.Covers.Hits() > 0 })
}

func TestPlaylistsSortChangesTheURLAndTheQuery(t *testing.T) {
	r := newGridRig(t, playlistsPath, nil)
	r.tt.Until("the playlists", r.tt.Shows("Focus"))
	if err := r.tt.Click("Sort"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("Title"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != playlistsPath+"?desc=false&sort=title" {
		t.Fatalf("after choosing Title the URL is %q", got)
	}
	want := []string{"Focus", "Road trip", "Sunday morning"}
	r.tt.Until("title order", func() bool { return slices.Equal(r.order(playlistNames...), want) })
	if asks := r.stub.asks(); !slices.Contains(asks, "playlists title desc=false offset=0") {
		t.Errorf("service asks %q", asks)
	}

	// Recently updated starts descending.
	if err := r.tt.Click("Sort"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.Click("Recently updated"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != playlistsPath+"?desc=true&sort=lastModified" {
		t.Fatalf("after choosing Recently updated the URL is %q", got)
	}
	r.tt.Until("the updated query", func() bool {
		return slices.Contains(r.stub.asks(), "playlists lastModified desc=true offset=0")
	})
}

func TestPlaylistsPageTakesItsOrderFromTheURL(t *testing.T) {
	r := newGridRig(t, playlistsPath+"?sort=title&desc=true", nil)
	want := []string{"Sunday morning", "Road trip", "Focus"}
	r.tt.Until("the reversed title order", func() bool { return slices.Equal(r.order(playlistNames...), want) })
	if asks := r.stub.asks(); asks[0] != "playlists title desc=true offset=0" {
		t.Errorf("first query %q", asks)
	}
	// A sort the page does not know falls back to the default.
	r2 := newGridRig(t, playlistsPath+"?sort=bogus", nil)
	r2.tt.Until("the default order", func() bool { return slices.Equal(r2.order(playlistNames...), playlistNames) })
}

func TestPlaylistTilesOpenWhereThePageExists(t *testing.T) {
	r := newGridRig(t, playlistsPath, nil)
	r.tt.Until("the playlists", r.tt.Shows("Focus"))
	if err := r.tt.Click("Focus"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/playlist/p.a1" {
		t.Errorf("Focus opened %q", got)
	}

	// Without the playlist page the tile is not a link.
	bare := newGridRig(t, playlistsPath, func(l *library, _ *gridStub) {
		l.env.HasRoute = func(string) bool { return false }
	})
	bare.tt.Until("the playlists", bare.tt.Shows("Focus"))
	_ = bare.tt.Click("Focus")
	if got := bare.router.Location(); got != playlistsPath {
		t.Errorf("a tile with no page navigated to %q", got)
	}
}

func TestPlaylistsGridKeyboardAndSelection(t *testing.T) {
	r := newGridRig(t, playlistsPath, nil)
	r.tt.Until("the playlists", r.tt.Shows("Road trip"))
	sel := &r.l.playlists.cover.st.Sel

	// Ctrl+click selects without opening, and focuses the tile.
	if err := r.tt.ClickWith(ui.Cmd, "Focus"); err != nil {
		t.Fatal(err)
	}
	if sel.Len() != 1 || r.router.Location() != playlistsPath {
		t.Fatalf("Ctrl+click: %d selected, location %q", sel.Len(), r.router.Location())
	}
	r.tt.Key(0, ui.KeyRight)
	if !r.tt.Focused("Road trip") {
		t.Errorf("Right did not move to the next tile")
	}
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Key(0, ui.KeyLeft)
	if !r.tt.Focused("Sunday morning") {
		t.Errorf("Left did not move to the first tile")
	}
	// Shift+arrows extend the selection from the anchor.
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(ui.Shift, ui.KeyRight)
	r.tt.Key(ui.Shift, ui.KeyRight)
	if sel.Len() != 3 {
		t.Errorf("Shift+Right twice selected %d tiles", sel.Len())
	}
	r.tt.Key(0, ui.KeyEscape)
	if sel.Len() != 0 {
		t.Errorf("Escape left %d selected", sel.Len())
	}
	// Shift+click selects the run to the tile.
	if err := r.tt.ClickWith(ui.Cmd, "Sunday morning"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.ClickWith(ui.Shift, "Road trip"); err != nil {
		t.Fatal(err)
	}
	if sel.Len() != 3 || r.router.Location() != playlistsPath {
		t.Errorf("Shift+click: %d selected, location %q", sel.Len(), r.router.Location())
	}
	// Enter opens the focused tile.
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(0, ui.KeyEnter)
	if got := r.router.Location(); !strings.HasPrefix(got, "/playlist/") {
		t.Errorf("Enter opened %q", got)
	}
}

func TestPlaylistsGateStates(t *testing.T) {
	t.Run("signed out", func(t *testing.T) {
		var opened bool
		r := newGridRig(t, playlistsPath, func(l *library, _ *gridStub) {
			l.openAccount = func() { opened = true }
		}, uitest.Empty())
		r.tt.Until("the sign-in text", r.tt.Shows("Sign in to see your playlists", "Playlists", "Sign in"))
		if r.tt.HasText("0 playlists") {
			t.Error("an empty library counts its playlists")
		}
		r.settle()
		if err := r.tt.Click("Sign in"); err != nil {
			t.Fatal(err)
		}
		r.tt.Drain()
		if !opened {
			t.Error("Sign in did not open the account dialog")
		}
	})
	t.Run("syncing", func(t *testing.T) {
		r := newGridRig(t, playlistsPath, func(l *library, s *gridStub) {
			fakeAuth(l, gridAuth{"signed_in"})
		}, uitest.Empty())
		r.tt.Until("the syncing text", r.tt.Shows("Your library is syncing",
			"Your playlists appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."))
	})
	t.Run("empty", func(t *testing.T) {
		r := newGridRig(t, playlistsPath, func(l *library, _ *gridStub) { fakeAuth(l, gridAuth{"signed_in"}) })
		// A synced library that holds no playlist.
		emptyTheSyncedLibrary(r.pageRig)
		r.tt.Until("the empty text", r.tt.Shows("No playlists in your library yet",
			"Playlists you make or add in Apple Music appear here after the next sync."))
	})
}

func TestPlaylistsErrorOffersTryAgain(t *testing.T) {
	r := newGridRig(t, playlistsPath, func(_ *library, s *gridStub) { s.fail(true) })
	r.tt.Until("the error", r.tt.Shows("Couldn't read your playlists from this computer", "Try again"))
	if r.tt.HasText("Focus") {
		t.Error("the error shows playlists")
	}
	r.stub.fail(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the playlists", r.tt.Shows("Focus", "3 playlists"))
}

func TestASyncRefreshesThePlaylists(t *testing.T) {
	r := newGridRig(t, playlistsPath, nil)
	r.tt.Until("the playlists", r.tt.Shows("3 playlists"))
	res, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO playlists (library_id, name, curator_name, updated_at) VALUES ('p.new', 'Brand New Mix', 'Sona Radio', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('playlist', ?, 99999999999, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	// Another domain finishing leaves the playlists alone.
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "songs", Done: 1, Total: 1, Finished: true})
	r.tt.Drain()
	if r.tt.HasText("Brand New Mix") {
		t.Fatal("the songs domain refetched the playlists")
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "playlists", Done: 4, Total: 4, Finished: true})
	// A curated playlist shows its curator.
	r.tt.Until("the new playlist", r.tt.Shows("Brand New Mix", "Sona Radio", "4 playlists"))
}

func TestPlaylistSubtitle(t *testing.T) {
	for _, c := range []struct {
		p    services.Playlist
		want string
	}{
		{services.Playlist{Curator: "Apple Music", TrackCount: 9}, "Apple Music"},
		{services.Playlist{TrackCount: 1}, "1 song"},
		{services.Playlist{TrackCount: 12}, "12 songs"},
		{services.Playlist{}, ""},
	} {
		if got := playlistSubtitle(&c.p); got != c.want {
			t.Errorf("subtitle of %+v = %q, want %q", c.p, got, c.want)
		}
	}
}
