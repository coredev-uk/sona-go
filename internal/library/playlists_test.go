package library

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
)

func folder(id, name string) map[string]any {
	return map[string]any{
		"id": id, "type": applemusic.TypeLibraryPlaylistFolders,
		"attributes": map[string]any{"name": name, "canDelete": true, "dateAdded": "2024-02-05T22:52:36Z"},
	}
}

// tombstone is what Apple keeps listing after a playlist or folder is
// deleted: no name, not deletable.
func tombstone(id, typ string) map[string]any {
	return map[string]any{
		"id": id, "type": typ,
		"attributes": map[string]any{"canDelete": false, "canEdit": false, "lastModifiedDate": "1970-01-01T00:00:00Z"},
	}
}

func treePlaylists(t *testing.T, e *env) map[string][2]any {
	t.Helper()
	rows, err := e.db.RQ.LibraryPlaylistTree(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][2]any{}
	for _, r := range rows {
		out[r.LibraryID.String] = [2]any{r.FolderID, r.FolderPosition}
	}
	return out
}

func TestSyncDropsTombstonesAndStoresFolders(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	fav := playlist("p.fav", "Favourite Songs", "2024-05-01T00:00:00Z")
	fav["attributes"].(map[string]any)["tags"] = []string{"favorited"}
	e.fake.playlists = append(e.fake.playlists, tombstone("p.gone", "library-playlists"))
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{
		folder("p.f1", "Mixes"),
		tombstone("p.fgone", applemusic.TypeLibraryPlaylistFolders),
		e.fake.playlists[0], // p.big
		fav,
	}
	e.fake.playlists = append(e.fake.playlists, fav)
	e.fake.folders["p.f1"] = []map[string]any{folder("p.f2", "Nested"), e.fake.playlists[2]} // p.small
	e.fake.folders["p.f2"] = []map[string]any{e.fake.playlists[1]}                           // p.empty
	if err := e.run(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c, _ := e.lib.Counts(ctx)
	if c.Playlists != 4 {
		t.Fatalf("playlists = %d, want 4 (tombstone dropped)", c.Playlists)
	}
	if _, _, err := e.lib.Playlist(ctx, "p.gone", ListOptions{}); err != ErrNotFound {
		t.Fatalf("tombstone stored: %v", err)
	}
	folders, err := e.db.RQ.ListPlaylistFolders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 2 || folders[0].LibraryID != "p.f1" || folders[0].ParentID != "" || folders[1].LibraryID != "p.f2" || folders[1].ParentID != "p.f1" {
		t.Fatalf("folders = %+v", folders)
	}
	got := treePlaylists(t, e)
	if got["p.big"] != [2]any{"", int64(1)} || got["p.small"] != [2]any{"p.f1", int64(1)} || got["p.empty"] != [2]any{"p.f2", int64(0)} {
		t.Fatalf("placement = %v", got)
	}
	if f, err := e.db.RQ.PlaylistFlags(ctx, db.NullString("p.fav")); err != nil || f.Favorited != 1 {
		t.Fatalf("favourite flag = %+v, %v", f, err)
	}

	// The playlist is deleted on Apple: it becomes a tombstone and the
	// next run removes it; an emptied folder disappears too.
	e.fake.playlists[1] = tombstone("p.empty", "library-playlists")
	delete(e.fake.folders, "p.f2")
	e.fake.folders["p.f1"] = e.fake.folders["p.f1"][1:]
	if err := e.run(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.lib.Playlist(ctx, "p.empty", ListOptions{}); err != ErrNotFound {
		t.Fatalf("deleted playlist still stored: %v", err)
	}
	if folders, _ := e.db.RQ.ListPlaylistFolders(ctx); len(folders) != 1 {
		t.Fatalf("folders after delete = %+v", folders)
	}
}

func TestStorePlaylistEdits(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	songs := func(ids ...string) []applemusic.Song {
		out := make([]applemusic.Song, len(ids))
		for i, id := range ids {
			out[i] = applemusic.Song{Resource: applemusic.Resource[applemusic.SongAttributes]{
				ID: id, Type: applemusic.TypeLibrarySongs,
				Attributes: applemusic.SongAttributes{Name: "S " + id, PlayParams: &applemusic.PlayParams{CatalogID: "c" + id}},
			}}
		}
		return out
	}
	p := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: "p.new", Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{Name: "New", CanEdit: true, CanDelete: true, LastModifiedDate: "2026-10-30T11:03:20Z"},
	}}
	// A new song (i.new) arrives with the playlist: it joins the library.
	if err := e.lib.StorePlaylist(ctx, p, "", songs("i.1", "i.new")); err != nil {
		t.Fatal(err)
	}
	pl, tr, err := e.lib.Playlist(ctx, "p.new", ListOptions{})
	if err != nil || pl.Name != "New" || len(tr) != 2 || tr[1].LibraryID != "i.new" {
		t.Fatalf("stored = %+v %+v %v", pl, tr, err)
	}
	if c, _ := e.lib.Counts(ctx); c.Songs != 21 || c.Playlists != 4 {
		t.Fatalf("counts = %+v", c)
	}
	if got := treePlaylists(t, e)["p.new"]; got[0] != "" {
		t.Fatalf("placement = %v", got)
	}
	// Reorder with a duplicate.
	if err := e.lib.StorePlaylistTracks(ctx, "p.new", "2026-10-30T11:04:00Z", songs("i.new", "i.1", "i.new")); err != nil {
		t.Fatal(err)
	}
	_, tr, _ = e.lib.Playlist(ctx, "p.new", ListOptions{})
	if len(tr) != 3 || tr[0].LibraryID != "i.new" || tr[2].LibraryID != "i.new" {
		t.Fatalf("reordered = %+v", tr)
	}
	if err := e.lib.MovePlaylist(ctx, "p.new", "p.folder"); err != nil {
		t.Fatal(err)
	}
	if n, _ := e.lib.FolderChildren(ctx, "p.folder"); n != 1 {
		t.Fatalf("folder children = %d", n)
	}
	if err := e.lib.DropPlaylist(ctx, "p.new"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.lib.Playlist(ctx, "p.new", ListOptions{}); err != ErrNotFound {
		t.Fatalf("dropped playlist: %v", err)
	}
	// The new song stays a library song.
	if _, err := e.lib.Song(ctx, "i.new"); err != nil {
		t.Fatal(err)
	}
}

// A move made here survives a sync while Apple's folder listing still
// shows the old place, and gives way once Apple lists it (or the grace
// period ends).
func TestSyncKeepsRecentFolderMove(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	big, small := e.fake.playlists[0], e.fake.playlists[2]
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), big, small}
	e.fake.folders["p.f1"] = []map[string]any{e.fake.playlists[1]}
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	e.sync.now = func() time.Time { return now }

	// Moved into Mixes here; Apple's listing lags.
	if err := e.lib.MovePlaylist(ctx, "p.big", "p.f1"); err != nil {
		t.Fatal(err)
	}
	e.sync.NoteMove("p.big", "p.f1")
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if got := treePlaylists(t, e)["p.big"]; got[0] != "p.f1" {
		t.Fatalf("lagging sync moved it back: %v", got)
	}

	// Apple lists it in the folder now: the note is done; a later move
	// on another device wins.
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), small}
	e.fake.folders["p.f1"] = []map[string]any{e.fake.playlists[1], big}
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	e.fake.folders[applemusic.PlaylistsRoot] = []map[string]any{folder("p.f1", "Mixes"), small, big}
	e.fake.folders["p.f1"] = []map[string]any{e.fake.playlists[1]}
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if got := treePlaylists(t, e)["p.big"]; got[0] != "" {
		t.Fatalf("confirmed move still pinned: %v", got)
	}

	// An unconfirmed move expires after the grace period.
	e.sync.NoteMove("p.small", "p.f1")
	now = now.Add(moveGrace + time.Minute)
	if err := e.runFull(t, DomainPlaylists); err != nil {
		t.Fatal(err)
	}
	if got := treePlaylists(t, e)["p.small"]; got[0] != "" {
		t.Fatalf("expired move kept: %v", got)
	}
}

// recentlyAdded is RecentlyAdded's page (as "album:l.0", "playlist:p.big")
// and its total.
func recentlyAdded(t *testing.T, e *env, o ListOptions) ([]string, int) {
	t.Helper()
	added, total, err := e.lib.RecentlyAdded(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range added {
		switch {
		case a.Album != nil:
			got = append(got, "album:"+a.Album.LibraryID)
		case a.Playlist != nil:
			got = append(got, "playlist:"+a.Playlist.LibraryID)
		}
	}
	return got, total
}

// #210: a dropped playlist leaves Recently added, its total included, and so
// does an entry whose album or playlist row is gone: the page lists and
// counts the same rows.
func TestDroppedPlaylistLeavesRecentlyAdded(t *testing.T) {
	e := newEnv(t)
	seed(e.fake, 20)
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	got, total := recentlyAdded(t, e, ListOptions{})
	if total != 5 || len(got) != total || !slices.Contains(got, "playlist:p.small") {
		t.Fatalf("before: %q, total %d", got, total)
	}

	if err := e.lib.DropPlaylist(ctx, "p.small"); err != nil {
		t.Fatal(err)
	}
	got, total = recentlyAdded(t, e, ListOptions{})
	if slices.Contains(got, "playlist:p.small") || total != 4 || len(got) != total {
		t.Fatalf("after DropPlaylist: %q, total %d", got, total)
	}

	// A library row whose playlist is gone (the row survived its target)
	// is neither listed nor counted, and paging still reaches the last
	// entry.
	if _, err := e.db.W.ExecContext(ctx, `DELETE FROM playlists WHERE library_id = 'p.big'`); err != nil {
		t.Fatal(err)
	}
	got, total = recentlyAdded(t, e, ListOptions{})
	if slices.Contains(got, "playlist:p.big") || total != 3 || len(got) != total {
		t.Fatalf("with a dangling row: %q, total %d", got, total)
	}
	var paged []string
	for off := 0; off < total; off++ {
		p, _ := recentlyAdded(t, e, ListOptions{Offset: off, Limit: 1})
		paged = append(paged, p...)
	}
	if !slices.Equal(paged, got) {
		t.Fatalf("one at a time: %q, want %q", paged, got)
	}
}
