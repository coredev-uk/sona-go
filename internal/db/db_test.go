package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

func openTemp(t *testing.T) (*DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sona test.db") // a space exercises URI escaping
	d, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d, path
}

func TestOpenPragmasAndReopen(t *testing.T) {
	ctx := context.Background()
	d, path := openTemp(t)
	var mode string
	var fk, busy int
	if err := d.W.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q, %v", mode, err)
	}
	if err := d.R.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("reader foreign_keys = %d, %v", fk, err)
	}
	if err := d.W.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busy); err != nil || busy != 5000 {
		t.Fatalf("busy_timeout = %d, %v", busy, err)
	}
	if _, err := d.R.ExecContext(ctx, "DELETE FROM accounts"); err == nil {
		t.Fatal("read pool accepted a write")
	}
	if err := d.Q.UpsertAccount(ctx, dbq.UpsertAccountParams{Storefront: "gb", Language: "en-GB", UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	d.Close()

	// Reopening runs no migration twice and keeps the data.
	d2, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	acc, err := d2.RQ.GetAccount(ctx)
	if err != nil || acc.Storefront != "gb" {
		t.Fatalf("account = %+v, %v", acc, err)
	}
}

func TestOpenRejectsMemory(t *testing.T) {
	if _, err := Open(context.Background(), ":memory:"); err == nil {
		t.Fatal("want error")
	}
}

func TestFTS5Available(t *testing.T) {
	ctx := context.Background()
	d, _ := openTemp(t)
	if _, err := d.W.ExecContext(ctx, `INSERT INTO search_fts (kind, ref, artwork, name, artist, album) VALUES ('song', 'i.1', '', 'Café del Mar', 'Energy 52', '')`); err != nil {
		t.Fatal(err)
	}
	var ref string
	if err := d.R.QueryRowContext(ctx, `SELECT ref FROM search_fts WHERE search_fts MATCH '"cafe"* "ener"*'`).Scan(&ref); err != nil || ref != "i.1" {
		t.Fatalf("prefix/diacritic match = %q, %v", ref, err)
	}
}

func TestCatalogAndLibraryRowsCoexist(t *testing.T) {
	ctx := context.Background()
	d, _ := openTemp(t)
	cat := dbq.UpsertCatalogTrackParams{CatalogID: NullString("100"), Name: "Song", UpdatedAt: 1}
	id1, err := d.Q.UpsertCatalogTrack(ctx, cat)
	if err != nil {
		t.Fatal(err)
	}
	cat.Name = "Song (renamed)"
	id2, err := d.Q.UpsertCatalogTrack(ctx, cat)
	if err != nil || id1 != id2 {
		t.Fatalf("catalog upsert ids %d %d, %v", id1, id2, err)
	}
	// Two library copies of the same catalog song are both allowed.
	for _, lib := range []string{"i.a", "i.b"} {
		if _, err := d.Q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{LibraryID: NullString(lib), CatalogID: NullString("100"), Name: "Song", UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := d.RQ.GetTrackByCatalogID(ctx, NullString("100"))
	if err != nil || !tr.LibraryID.Valid {
		t.Fatalf("GetTrackByCatalogID should prefer the library row: %+v, %v", tr, err)
	}
}

func TestWipeUserData(t *testing.T) {
	ctx := context.Background()
	d, _ := openTemp(t)
	q := d.Q
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	tid, err := q.UpsertLibraryTrack(ctx, dbq.UpsertLibraryTrackParams{LibraryID: NullString("i.1"), Name: "Mine", UpdatedAt: 1})
	must(err)
	must(q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: tid, Generation: 1}))
	pid, err := q.UpsertLibraryPlaylist(ctx, dbq.UpsertLibraryPlaylistParams{LibraryID: NullString("p.1"), Name: "Mix", UpdatedAt: 1})
	must(err)
	must(q.InsertPlaylistTrack(ctx, dbq.InsertPlaylistTrackParams{PlaylistID: pid, Position: 0, TrackID: tid}))
	_, err = q.UpsertCatalogTrack(ctx, dbq.UpsertCatalogTrackParams{CatalogID: NullString("9"), Name: "Theirs", UpdatedAt: 1})
	must(err)
	must(q.PutCache(ctx, dbq.PutCacheParams{Key: "gb|album|1", Storefront: "gb", Body: []byte("{}"), FetchedAt: 1, ExpiresAt: 2}))
	must(q.UpsertAccount(ctx, dbq.UpsertAccountParams{Storefront: "gb", UpdatedAt: 1}))
	must(q.UpsertFavorite(ctx, dbq.UpsertFavoriteParams{Kind: "songs", AppleID: "9", Value: 1, UpdatedAt: 1}))
	must(q.UpsertPin(ctx, dbq.UpsertPinParams{Kind: "albums", AppleID: "1", PinnedAt: 1}))
	_, err = q.InsertHistory(ctx, dbq.InsertHistoryParams{PlayedAt: 1, ItemID: "9"})
	must(err)
	must(q.StartSync(ctx, dbq.StartSyncParams{Domain: "songs", Generation: 1}))

	must(d.WipeUserData(ctx))

	for table, want := range map[string]int{
		"tracks": 1, "catalog_cache": 1, "playlists": 0, "playlist_tracks": 0, "library_items": 0,
		"accounts": 0, "favorites": 0, "pins": 0, "playback_history": 0, "sync_state": 0,
	} {
		var n int
		must(d.R.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n))
		if n != want {
			t.Errorf("%s: %d rows after wipe, want %d", table, n, want)
		}
	}
	if _, err := d.RQ.GetTrackByLibraryID(ctx, NullString("i.1")); err != sql.ErrNoRows {
		t.Fatalf("library track survived: %v", err)
	}
}
