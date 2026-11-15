package library

import (
	"net/http"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// slowApple holds Apple's answer to a DELETE until gate is closed, then
// answers status (204 when 0): the time between a confirmed deletion and
// Apple's answer.
type slowApple struct {
	gate   chan struct{}
	status int

	mu      sync.Mutex
	arrived int
}

func (a *slowApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
		return
	}
	a.mu.Lock()
	a.arrived++
	a.mu.Unlock()
	<-a.gate
	if a.status != 0 {
		w.WriteHeader(a.status)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *slowApple) deletes() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.arrived
}

// cascadeRig is the library with an album of one song ("Solo") and one of two
// ("Pair"), over an Apple that holds its DELETE.
func cascadeRig(t *testing.T, status int) (*gridRig, *slowApple) {
	t.Helper()
	apple := &slowApple{gate: make(chan struct{}), status: status}
	r := newGridRigH(t, albumsPath, 900, nil,
		uitest.Albums(
			fixture.Album{Name: "Solo", Artist: "Ana", Titles: []string{"Only Track"}},
			fixture.Album{Name: "Pair", Artist: "Ben"},
		), uitest.Apple(apple))
	t.Cleanup(func() {
		select {
		case <-apple.gate:
		default:
			close(apple.gate)
		}
	})
	r.tt.Until("the albums", r.tt.Shows("Solo", "Pair"))
	return r, apple
}

// deleteSong confirms the deletion of a seeded song of album i, track j.
func deleteSong(t *testing.T, r *gridRig, i, j int, name string) {
	t.Helper()
	id := fixture.SongLibraryID(i, j)
	r.l.env.ItemStates.Remove(kit.StateSongs, kit.StateItem{ID: id, Source: services.SourceLibrary, LibraryID: id}, name)
	r.tt.Drain()
	if err := r.tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
}

// goes waits for the texts to leave the page, with a reason.
func (r *gridRig) goes(why string, texts ...string) {
	r.tt.T.Helper()
	r.tt.Until(why, func() bool {
		for _, w := range texts {
			if r.tt.HasText(w) {
				return false
			}
		}
		return true
	})
}

// #350: deleting an album's last song takes the album out of Albums, Recently
// Added and Songs at once, before Apple answers; the album with songs left
// stays.
func TestDeletingTheLastSongDropsItsAlbumAtOnce(t *testing.T) {
	r, apple := cascadeRig(t, 0)
	deleteSong(t, r, 0, 0, "Only Track")
	r.tt.Until("Apple asked", func() bool { return apple.deletes() == 1 })
	// Apple has not answered (its DELETE is held), yet the album is gone.
	r.goes("Solo gone from Albums", "Solo")
	if !r.tt.HasText("Pair") {
		t.Error("the album with songs left went too")
	}
	r.router.Push(recentPath)
	r.tt.Until("Recently added without Solo", r.tt.Shows("Pair"))
	r.goes("Solo gone from Recently added", "Solo")
	r.router.Push(songsPath)
	r.tt.Until("the songs", r.tt.Shows("Pair song 1"))
	r.goes("the song gone from Songs", "Only Track")
	close(apple.gate)
	r.tt.Until("the toast", r.tt.Shows("Deleted “Only Track” from your library"))
	r.tt.Drain()
	// Apple's answer changes nothing the page shows (the toast names the song).
	for _, w := range r.tt.Texts() {
		if w == "Only Track" {
			t.Error("the song came back")
		}
	}
}

// Deleting one of two songs leaves the album, which shows the song count of
// what is left.
func TestDeletingASongKeepsAnAlbumWithSongsLeft(t *testing.T) {
	r, apple := cascadeRig(t, 0)
	r.router.Push(songsPath)
	r.tt.Until("the songs", r.tt.Shows("Pair song 1", "Pair song 2", "Only Track"))
	deleteSong(t, r, 1, 0, "Pair song 1")
	r.goes("the song gone from Songs", "Pair song 1")
	if !r.tt.HasText("Pair song 2") {
		t.Error("the other song went too")
	}
	r.router.Push(albumsPath)
	r.tt.Until("the albums", r.tt.Shows("Solo", "Pair"))
	close(apple.gate)
	r.tt.Until("the toast", r.tt.Shows("Deleted “Pair song 1” from your library"))
}

// A refused DELETE puts the album and the song back, and says so.
func TestAFailedDeleteRestoresTheSongAndItsAlbum(t *testing.T) {
	r, apple := cascadeRig(t, http.StatusBadRequest)
	deleteSong(t, r, 0, 0, "Only Track")
	r.tt.Until("Apple asked", func() bool { return apple.deletes() == 1 })
	r.goes("Solo gone while Apple works", "Solo")
	close(apple.gate)
	r.tt.Until("the toast and the album back", r.tt.Shows("Couldn't delete “Only Track”", "Solo", "Pair"))
	r.router.Push(recentPath)
	r.tt.Until("Recently added with Solo", r.tt.Shows("Solo", "Pair"))
	r.router.Push(songsPath)
	r.tt.Until("Songs with the song", r.tt.Shows("Only Track"))
}
