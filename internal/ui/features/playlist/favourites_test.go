package playlist

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// Apple's Favourite Songs playlist is Library → Favourite songs here: the
// sidebar lists it nowhere and a way to its page ends on that page.

func favouritedTree() services.PlaylistTree {
	tree := sbLibrary()
	fav := treePlaylist("p.fav", "Favourite Songs", "", 2, false)
	fav.Favorited = true
	tree.Playlists = append(tree.Playlists, fav)
	return tree
}

func TestSidebarDoesNotListTheFavouriteSongsPlaylist(t *testing.T) {
	e := sbSetup(t, favouritedTree(), nil)
	e.shows("Apple Music", "Quiet Hours", "Road Trip")
	if e.tt.HasText("Favourite Songs") {
		t.Error("the Favourite Songs playlist is in the sidebar")
	}
}

func TestAPinOfTheFavouriteSongsPlaylistOpensLibraryFavouriteSongs(t *testing.T) {
	pin := services.Pin{Kind: "playlist", ID: "p.fav", Name: "Faves", Action: "drillIn"}
	e := sbSetup(t, favouritedTree(), []services.Pin{pin})
	e.shows("Pinned", "Faves", "Quiet Hours")
	if err := e.tt.Click("Faves"); err != nil {
		t.Fatal(err)
	}
	if got := e.router.Path(); got != favouritesPath {
		t.Errorf("the pin went to %q, want %q", got, favouritesPath)
	}
}

func TestTheFavouriteSongsPlaylistPageRedirectsToLibraryFavouriteSongs(t *testing.T) {
	gate := make(chan struct{})
	r := newPageRigWith(t, "p.fav", func(a *fakeAPI) { a.tree, a.treeGate = favouritedTree(), gate },
		services.PlaylistDetail{Playlist: services.Playlist{ID: "p.fav", Name: "Favourite Songs"}})
	shown := false
	r.tt.Until("the tree to be asked", func() bool {
		shown = shown || r.tt.HasText("Favourite Songs")
		return r.api.did("Tree")
	})
	r.tt.Until("a few frames", func() bool { shown = shown || r.tt.HasText("Favourite Songs"); return true })
	if r.api.did("Playlist p.fav") {
		t.Error("the playlist was fetched while the tree was loading")
	}
	close(gate)
	r.tt.Until("the redirect", func() bool {
		shown = shown || r.tt.HasText("Favourite Songs")
		return r.router.Path() == favouritesPath
	})
	if shown {
		t.Error("the playlist page was drawn before the redirect")
	}
	if r.router.CanGoBack() {
		t.Error("the redirect pushed a history entry instead of replacing the page")
	}
}

func TestAnotherPlaylistPageIsNotRedirected(t *testing.T) {
	r := newPageRigWith(t, "p.ed", func(a *fakeAPI) { a.tree = favouritedTree() },
		services.PlaylistDetail{Playlist: services.Playlist{ID: "p.ed", Name: "Quiet Hours"}})
	r.shows("Quiet Hours")
	if got := r.router.Path(); got != "/playlist/p.ed" {
		t.Errorf("path %q", got)
	}
}
