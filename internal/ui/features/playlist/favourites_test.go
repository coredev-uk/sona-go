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
	e.shows("Pinned", "Faves")
	if err := e.tt.Click("Faves"); err != nil {
		t.Fatal(err)
	}
	if got := e.router.Path(); got != favouritesPath {
		t.Errorf("the pin went to %q, want %q", got, favouritesPath)
	}
}

func TestTheFavouriteSongsPlaylistPageRedirectsToLibraryFavouriteSongs(t *testing.T) {
	r := newPageRigWith(t, "p.fav", func(a *fakeAPI) { a.tree = favouritedTree() },
		services.PlaylistDetail{Playlist: services.Playlist{ID: "p.fav", Name: "Favourite Songs"}, Favorited: true})
	r.tt.Until("the redirect", func() bool { return r.router.Path() == favouritesPath })
	if r.tt.HasText("Favourite Songs") {
		t.Error("the playlist page showed before the redirect")
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
