package library

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// listedInOrder is the library IDs of the playlists of folder, as the
// sidebar sorts them: the user's order first, then Apple's.
func listedInOrder(t *testing.T, e *env, folder string) []string {
	t.Helper()
	ids, err := e.db.RQ.FolderPlaylistOrder(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String
	}
	return out
}

// orders are the places the user gave the playlists, by library ID (0: none).
func orders(t *testing.T, e *env) map[string]int {
	t.Helper()
	_, placed, err := e.lib.PlaylistTree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, p := range placed {
		out[p.LibraryID] = p.Order
	}
	return out
}

func TestReorderPlaylistKeepsTheUsersOrderThroughASync(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	big, empty, small := e.fake.playlists[0], e.fake.playlists[1], e.fake.playlists[2]
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), big, empty, small}
	e.fake.folders["p.f1"] = nil
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if got := orders(t, e); got["p.big"] != 0 || got["p.empty"] != 0 || got["p.small"] != 0 {
		t.Fatalf("a playlist has an order before the user gave one: %v", got)
	}
	if got, want := listedInOrder(t, e, ""), []string{"p.big", "p.empty", "p.small"}; !slices.Equal(got, want) {
		t.Fatalf("Apple's order = %v, want %v", got, want)
	}

	// Before another playlist, then last.
	if err := e.lib.ReorderPlaylist(ctx, "p.small", "p.big"); err != nil {
		t.Fatal(err)
	}
	if got, want := listedInOrder(t, e, ""), []string{"p.small", "p.big", "p.empty"}; !slices.Equal(got, want) {
		t.Fatalf("after placing small first = %v, want %v", got, want)
	}
	if err := e.lib.ReorderPlaylist(ctx, "p.small", ""); err != nil {
		t.Fatal(err)
	}
	if got, want := listedInOrder(t, e, ""), []string{"p.big", "p.empty", "p.small"}; !slices.Equal(got, want) {
		t.Fatalf("after placing small last = %v, want %v", got, want)
	}
	if err := e.lib.ReorderPlaylist(ctx, "p.empty", "p.big"); err != nil {
		t.Fatal(err)
	}
	// Before itself changes nothing.
	if err := e.lib.ReorderPlaylist(ctx, "p.empty", "p.empty"); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"p.empty": 1, "p.big": 2, "p.small": 3}
	if got := orders(t, e); got["p.empty"] != 1 || got["p.big"] != 2 || got["p.small"] != 3 {
		t.Fatalf("orders = %v, want %v", got, want)
	}

	// Apple lists them another way round on the next sync (another device,
	// or none: the sync rewrites its own positions): the user's order stays.
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), small, big, empty}
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if got, want := listedInOrder(t, e, ""), []string{"p.empty", "p.big", "p.small"}; !slices.Equal(got, want) {
		t.Fatalf("after a sync = %v, want the user's order %v", got, want)
	}
}

func TestReorderPlaylistRefusesWhatIsNotInTheFolder(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	big, empty, small := e.fake.playlists[0], e.fake.playlists[1], e.fake.playlists[2]
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), big, small}
	e.fake.folders["p.f1"] = []map[string]any{empty}
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, c := range []struct{ id, before string }{
		{"p.big", "p.empty"}, // in another folder
		{"p.big", "p.nope"},
		{"p.nope", ""},
	} {
		if err := e.lib.ReorderPlaylist(ctx, c.id, c.before); !errors.Is(err, ErrNotFound) {
			t.Errorf("ReorderPlaylist(%q, %q) = %v, want ErrNotFound", c.id, c.before, err)
		}
	}
	if got := orders(t, e); got["p.big"] != 0 || got["p.small"] != 0 {
		t.Errorf("a refused reorder left orders: %v", got)
	}
}

func TestAMovedPlaylistHasNoPlaceUntilItIsPlaced(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	big, empty, small := e.fake.playlists[0], e.fake.playlists[1], e.fake.playlists[2]
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), big, small}
	e.fake.folders["p.f1"] = []map[string]any{empty}
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := e.lib.ReorderPlaylist(ctx, "p.small", "p.big"); err != nil {
		t.Fatal(err)
	}
	// Into the folder, and back: the old place does not come back.
	if err := e.lib.MovePlaylist(ctx, "p.small", "p.f1"); err != nil {
		t.Fatal(err)
	}
	if got := orders(t, e); got["p.small"] != 0 {
		t.Fatalf("a playlist keeps its place after a move: %v", got)
	}
	if got, want := listedInOrder(t, e, "p.f1"), []string{"p.empty", "p.small"}; !slices.Equal(got, want) {
		t.Fatalf("the folder lists %v, want %v", got, want)
	}
	if err := e.lib.MovePlaylist(ctx, "p.small", ""); err != nil {
		t.Fatal(err)
	}
	if got, want := listedInOrder(t, e, ""), []string{"p.big", "p.small"}; !slices.Equal(got, want) {
		t.Fatalf("the top level lists %v, want %v", got, want)
	}
}
