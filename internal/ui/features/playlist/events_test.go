package playlist

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// countOf is how many times the fake was asked call.
func countOf(api *fakeAPI, call string) int {
	n := 0
	for _, c := range api.asked() {
		if c == call {
			n++
		}
	}
	return n
}

// untilCount drains what was posted until the fake was asked call n times.
func untilCount(t *testing.T, h interface{ Drain() int }, api *fakeAPI, call string, n int) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		h.Drain()
		if countOf(api, call) >= n {
			return
		}
	}
	t.Fatalf("%q asked %d times, want %d (calls %v)", call, countOf(api, call), n, api.asked())
}

// settleQueries lets the queries answer: the views of the sidebar and the
// page ask every frame in the app; here the test does.
func settleQueries(t *testing.T, p *playlists, h interface{ Drain() int }, id string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(2 * time.Millisecond) {
		h.Drain()
		if p.tree().HasData && p.pins().HasData && (id == "" || p.detail(id).HasData) {
			return
		}
	}
	t.Fatal("the queries did not answer")
}

// playlists:changed is how the sidebar and the page learn of an edit
// (index.ts's eventHandlers): the tree, counts and library playlists when
// the tree changed, the pins when they did, the changed playlists' details.
func TestPlaylistsChangedRefreshesWhatChanged(t *testing.T) {
	p, api, _, h, _ := testPlaylists(t, "/")
	api.tree = services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.1", "Road Trip", "", 0, true)}}
	api.details["p.1"] = libraryDetail("p.1", "Road Trip", threeSongs()...)
	settleQueries(t, p, h.Main, "p.1")
	tree, pins, detail := countOf(api, "Tree"), countOf(api, "Pins"), countOf(api, "Playlist p.1")

	// A new playlist: the tree reloads and shows it.
	api.mu.Lock()
	api.tree.Playlists = append(api.tree.Playlists, treePlaylist("p.2", "Commute", "", 1, true))
	api.mu.Unlock()
	h.Bus.Emit(services.EventPlaylistsChanged, services.PlaylistsChanged{Tree: true})
	untilCount(t, h.Main, api, "Tree", tree+1)
	for deadline := time.Now().Add(5 * time.Second); len(p.tree().Data.Playlists) != 2; {
		if time.Now().After(deadline) {
			t.Fatal("the new playlist never reached the tree")
		}
		h.Main.Drain()
		time.Sleep(2 * time.Millisecond)
	}
	if countOf(api, "Pins") != pins || countOf(api, "Playlist p.1") != detail {
		t.Error("a tree change refreshed the pins or a detail")
	}

	// Pins.
	h.Bus.Emit(services.EventPlaylistsChanged, services.PlaylistsChanged{Pins: true})
	untilCount(t, h.Main, api, "Pins", pins+1)

	// A playlist's own change reloads its detail.
	h.Bus.Emit(services.EventPlaylistsChanged, services.PlaylistsChanged{IDs: []string{"p.1"}})
	untilCount(t, h.Main, api, "Playlist p.1", detail+1)
}

// A playlist being edited here keeps its optimistic state: its own answer
// replaces it, not a refresh in between.
func TestPlaylistsChangedSkipsAPlaylistBeingEdited(t *testing.T) {
	p, api, _, h, _ := testPlaylists(t, "/")
	api.details["p.1"] = libraryDetail("p.1", "Road Trip", threeSongs()...)
	api.tree = services.PlaylistTree{}
	settleQueries(t, p, h.Main, "p.1")
	detail := countOf(api, "Playlist p.1")

	gate := make(chan struct{})
	api.setGate(gate)
	p.moveTrack("p.1", 0, 2, threeSongs()[0])
	untilCount(t, h.Main, api, "MoveTrack p.1 0 2 a", 1)
	h.Bus.Emit(services.EventPlaylistsChanged, services.PlaylistsChanged{IDs: []string{"p.1"}})
	for range 20 {
		h.Main.Drain()
		time.Sleep(2 * time.Millisecond)
	}
	if got := countOf(api, "Playlist p.1"); got != detail {
		t.Errorf("the detail was refetched %d times while its edit was out", got-detail)
	}
	close(gate)
}

// The optimistic order survives an answer that arrives after a newer edit
// was made: the older answer must not put the first move's result over it.
func TestAnOlderAnswerDoesNotOverwriteANewerEdit(t *testing.T) {
	p, api, _, h, _ := testPlaylists(t, "/")
	api.details["p.1"] = libraryDetail("p.1", "Road Trip", threeSongs()...)
	api.tree = services.PlaylistTree{}
	settleQueries(t, p, h.Main, "p.1")
	songs := threeSongs()

	p.moveTrack("p.1", 0, 1, songs[0]) // A moves down: b a c
	// The answer is posted but not yet run on the main thread.
	for deadline := time.Now().Add(5 * time.Second); !h.Main.Pending(); time.Sleep(2 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the first edit never answered: %v", api.asked())
		}
	}
	// The user edits again first; this edit stays out (gate) so only the
	// older answer runs.
	gate := make(chan struct{})
	api.setGate(gate)
	p.moveTrack("p.1", 1, 2, songs[0]) // b c a
	h.Main.Drain()
	var got []string
	for _, s := range p.detail("p.1").Data.Tracks {
		got = append(got, s.ID)
	}
	if !slices.Equal(got, []string{"b", "c", "a"}) {
		t.Errorf("tracks %s after the older answer, want the newer edit's b c a", strings.Join(got, " "))
	}
	close(gate)
}
