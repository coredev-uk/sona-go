package library

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

func TestRecentlyAddedShowsAlbumsNewestFirst(t *testing.T) {
	r := newGridRig(t, recentPath, nil, uitest.Albums(sevenAlbums...))
	r.tt.Until("the entries", r.tt.Shows("Recently added", "7 albums and playlists", "Paper Planes", "Zebra Crossing", "Di", "Mia"))
	want := []string{"Paper Planes", "Single Malt", "Blue Hour", "Mango Season", "Lone Wolf", "Apple Orchard", "Zebra Crossing"}
	if got := r.order(names(sevenAlbums)...); !slices.Equal(got, want) {
		t.Errorf("entries shown %q, want %q", got, want)
	}
	// There is no sort control: the order is when they joined.
	if r.tt.HasText("Sort") {
		t.Error("the page has a sort control")
	}
	if asks := r.stub.asks(); len(asks) == 0 || asks[0] != "recent offset=0" {
		t.Errorf("first query %q", asks)
	}
	r.tt.Until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
}

func TestRecentlyAddedMixesAlbumsAndPlaylists(t *testing.T) {
	r := newGridRig(t, recentPath, nil)
	// 24 albums and 3 playlists; a playlist without a curator says so.
	r.tt.Until("the entries", r.tt.Shows("27 albums and playlists", "Playlist", "Fieldnotes"))
	for _, p := range playlistNames {
		if !r.tt.HasText(p) {
			t.Errorf("playlist %q is missing", p)
		}
	}
	// Neither kind of tile has a play control (v0.2.0's CoverTile sets none).
	for _, p := range append([]string{"Fieldnotes"}, playlistNames...) {
		if _, ok := r.tt.Find("Play " + p); ok {
			t.Errorf("%q has a play control", p)
		}
	}
}

func TestRecentlyAddedSingularHeader(t *testing.T) {
	r := newGridRig(t, recentPath, nil, uitest.Albums(fixture.Album{Name: "Only Album", Artist: "X"}))
	r.tt.Until("the entry", r.tt.Shows("Only Album", "1 album or playlist"))
}

func TestRecentlyAddedOpensAnAlbumAndHasNoPlayButton(t *testing.T) {
	r := newGridRig(t, recentPath, nil, uitest.Albums(sevenAlbums...))
	r.tt.Until("the entries", r.tt.Shows("Mango Season"))
	if _, ok := r.tt.Find("Play Mango Season"); ok {
		t.Error("an album tile has a play button")
	}
	if err := r.tt.Click("Blue Hour"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); !strings.HasPrefix(got, "/album/") {
		t.Errorf("an album tile opened %q", got)
	}
}

func TestRecentlyAddedPlaylistTileOpensItsPage(t *testing.T) {
	r := newGridRigH(t, recentPath, 1800, nil)
	r.tt.Until("the entries", r.tt.Shows("27 albums and playlists", "Road trip"))
	if err := r.tt.Click("Road trip"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/playlist/p.a0" {
		t.Errorf("a playlist tile opened %q", got)
	}
}

func TestRecentlyAddedTilesLinkOnlyToExistingPages(t *testing.T) {
	r := newGridRig(t, recentPath, func(l *library, _ *gridStub) {
		l.env.HasRoute = func(string) bool { return false }
	}, uitest.Albums(sevenAlbums...))
	r.tt.Until("the entries", r.tt.Shows("Blue Hour"))
	_ = r.tt.Click("Blue Hour")
	if got := r.router.Location(); got != recentPath {
		t.Errorf("a tile with no page navigated to %q", got)
	}
}

func TestRecentlyAddedKeyboardAndSelection(t *testing.T) {
	r := newGridRig(t, recentPath, nil, uitest.Albums(sevenAlbums...))
	r.tt.Until("the entries", r.tt.Shows("Paper Planes", "Blue Hour"))
	sel := &r.l.recent.cover.st.Sel
	if err := r.tt.ClickWith(ui.Cmd, "Paper Planes"); err != nil {
		t.Fatal(err)
	}
	if sel.Len() != 1 || r.router.Location() != recentPath {
		t.Fatalf("Ctrl+click: %d selected, location %q", sel.Len(), r.router.Location())
	}
	r.tt.Key(0, ui.KeyRight)
	if !r.tt.Focused("Single Malt") {
		t.Errorf("Right did not move to the next tile")
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(ui.Shift, ui.KeyRight)
	r.tt.Key(ui.Shift, ui.KeyRight)
	if sel.Len() != 3 {
		t.Errorf("Shift+Right twice selected %d tiles", sel.Len())
	}
	r.tt.Key(0, ui.KeyEscape)
	if err := r.tt.ClickWith(ui.Cmd, "Paper Planes"); err != nil {
		t.Fatal(err)
	}
	if err := r.tt.ClickWith(ui.Shift, "Blue Hour"); err != nil {
		t.Fatal(err)
	}
	if sel.Len() != 3 || r.router.Location() != recentPath {
		t.Errorf("Shift+click: %d selected, location %q", sel.Len(), r.router.Location())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Key(0, ui.KeyEnter)
	if got := r.router.Location(); !strings.HasPrefix(got, "/album/") {
		t.Errorf("Enter opened %q", got)
	}
}

func TestRecentlyAddedGateStates(t *testing.T) {
	t.Run("signed out", func(t *testing.T) {
		var opened bool
		r := newGridRig(t, recentPath, func(l *library, _ *gridStub) {
			l.openAccount = func() { opened = true }
		}, uitest.Empty())
		r.tt.Until("the sign-in text", r.tt.Shows("Sign in to see your recent additions", "Recently added"))
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
		r := newGridRig(t, recentPath, func(l *library, s *gridStub) {
			fakeAuth(l, gridAuth{"signed_in"})
		}, uitest.Empty())
		r.tt.Until("the syncing text", r.tt.Shows("Your library is syncing",
			"Your recent additions appear here as Sona copies them from Apple Music. Large libraries take a minute or two the first time."))
	})
	t.Run("empty", func(t *testing.T) {
		r := newGridRig(t, recentPath, func(l *library, _ *gridStub) { fakeAuth(l, gridAuth{"signed_in"}) })
		// A synced library that holds no album or playlist.
		if _, err := r.h.DB.W.ExecContext(t.Context(), `DELETE FROM library_items WHERE kind IN ('album', 'playlist')`); err != nil {
			t.Fatal(err)
		}
		r.l.env.Queries.Invalidate(store.KeyLibrary)
		r.tt.Until("the empty text", r.tt.Shows("No recent additions in your library yet",
			"Albums and playlists you add in Apple Music appear here, newest first."))
	})
}

func TestRecentlyAddedErrorOffersTryAgain(t *testing.T) {
	r := newGridRig(t, recentPath, func(_ *library, s *gridStub) { s.fail(true) })
	r.tt.Until("the error", r.tt.Shows("Couldn't read your recent additions from this computer", "Try again"))
	r.stub.fail(false)
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the entries", r.tt.Shows("27 albums and playlists"))
}

func TestASyncRefreshesRecentlyAdded(t *testing.T) {
	r := newGridRig(t, recentPath, nil, uitest.Albums(sevenAlbums[:2]...))
	r.tt.Until("the entries", r.tt.Shows("2 albums and playlists"))
	add := func(domain string) {
		t.Helper()
		res, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO albums (library_id, catalog_id, name, artist_name, updated_at) VALUES ('l.new', '999', 'Brand New', '', 0)`)
		if err != nil {
			t.Fatal(err)
		}
		ref, _ := res.LastInsertId()
		if _, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('album', ?, 99999999999, 1)`, ref); err != nil {
			t.Fatal(err)
		}
		r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: domain, Done: 3, Total: 3, Finished: true})
	}
	// The albums domain finishing refreshes it.
	add("albums")
	r.tt.Until("the new album", r.tt.Shows("Brand New", "3 albums and playlists"))

	// So does the playlists domain.
	res, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO playlists (library_id, name, curator_name, updated_at) VALUES ('p.new', 'Fresh Mix', 'Sona Radio', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := res.LastInsertId()
	if _, err := r.h.DB.W.ExecContext(t.Context(), `INSERT INTO library_items (kind, ref, date_added, generation) VALUES ('playlist', ?, 99999999999, 1)`, ref); err != nil {
		t.Fatal(err)
	}
	r.h.Env.Bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "playlists", Done: 4, Total: 4, Finished: true})
	r.tt.Until("the new playlist", r.tt.Shows("Fresh Mix", "Sona Radio", "4 albums and playlists"))
}

// #210: a playlist deleted in Sona leaves the page at once. The delete
// removes its library row and says so with PlaylistsChanged{Tree: true}; the
// page must not wait for the next sync to ask again.
func TestADeletedPlaylistLeavesRecentlyAdded(t *testing.T) {
	r := newGridRigH(t, recentPath, 1800, nil)
	r.tt.Until("the entries", r.tt.Shows("27 albums and playlists", "Road trip"))
	before := len(r.stub.asks())
	// What Library.DropPlaylist leaves behind for "Road trip" (p.a0).
	if _, err := r.h.DB.W.ExecContext(t.Context(), `DELETE FROM library_items
		WHERE kind = 'playlist' AND ref = (SELECT id FROM playlists WHERE library_id = 'p.a0')`); err != nil {
		t.Fatal(err)
	}
	r.h.Env.Bus.Emit(services.EventPlaylistsChanged, services.PlaylistsChanged{IDs: []string{"p.a0"}, Tree: true})
	r.tt.Until("the page without the playlist", func() bool {
		return r.tt.HasText("26 albums and playlists") && !r.tt.HasText("Road trip")
	})
	if asks := r.stub.asks(); len(asks) <= before || asks[len(asks)-1] != "recent offset=0" {
		t.Errorf("the page did not ask again: %q", asks)
	}
}
