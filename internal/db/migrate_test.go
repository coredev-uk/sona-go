package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// openRaw opens a bare SQLite file for migration tests (no migrations run).
func openRaw(t *testing.T, name string) *sql.DB {
	t.Helper()
	// No fsync: the schema each migration leaves is the test, and a database
	// per version would otherwise sync on every statement.
	w, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), name)+"?_foreign_keys=1&_synchronous=OFF")
	if err != nil {
		t.Fatal(err)
	}
	w.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// schema returns every schema object's SQL, sorted by name.
func schema(t *testing.T, w *sql.DB) string {
	t.Helper()
	rows, err := w.Query(`SELECT type || ' ' || name || ': ' || coalesce(sql, '') FROM sqlite_master
		WHERE name NOT LIKE 'goose%' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var b strings.Builder
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		b.WriteString(s + "\n")
	}
	return b.String()
}

func TestMigrationsAreDense(t *testing.T) {
	p, err := provider(openRaw(t, "dense.db"))
	if err != nil {
		t.Fatal(err)
	}
	for i, src := range p.ListSources() {
		if src.Version != int64(i+1) {
			t.Fatalf("migration %s has version %d, want %d: number migrations densely", filepath.Base(src.Path), src.Version, i+1)
		}
	}
}

// TestMigrateFromEveryVersion applies the migrations to an empty database
// and to a database left at every earlier version; each ends at the latest
// version with the same schema.
func TestMigrateFromEveryVersion(t *testing.T) {
	ctx := context.Background()
	fresh := openRaw(t, "fresh.db")
	if err := migrate(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	want := schema(t, fresh)
	p, _ := provider(fresh)
	latest, err := p.GetDBVersion(ctx)
	if err != nil || latest != int64(len(p.ListSources())) {
		t.Fatalf("fresh database at version %d (%v), want %d", latest, err, len(p.ListSources()))
	}

	for v := range latest {
		w := openRaw(t, "from.db")
		p, err := provider(w)
		if err != nil {
			t.Fatal(err)
		}
		if v > 0 {
			if _, err := p.UpTo(ctx, v); err != nil {
				t.Fatalf("up to %d: %v", v, err)
			}
		}
		if err := migrate(ctx, w); err != nil {
			t.Fatalf("migrate from version %d: %v", v, err)
		}
		if got, _ := p.GetDBVersion(ctx); got != latest {
			t.Fatalf("from version %d: ended at %d, want %d", v, got, latest)
		}
		if got := schema(t, w); got != want {
			t.Fatalf("from version %d: schema differs from a fresh database:\n%s\nwant:\n%s", v, got, want)
		}
	}
}

// TestMigrateMarksEarlierSyncsFull checks 00005 on data: every sync before
// it was a full run, so a completed domain's last_full is its last success.
func TestMigrateMarksEarlierSyncsFull(t *testing.T) {
	ctx := context.Background()
	w := openRaw(t, "sync.db")
	p, err := provider(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO sync_state (domain, last_success) VALUES ('songs', 1234), ('albums', NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	var songs, albums sql.NullInt64
	if err := w.QueryRow(`SELECT (SELECT last_full FROM sync_state WHERE domain = 'songs'), (SELECT last_full FROM sync_state WHERE domain = 'albums')`).Scan(&songs, &albums); err != nil {
		t.Fatal(err)
	}
	if songs.Int64 != 1234 || albums.Valid {
		t.Fatalf("last_full = %v, %v", songs, albums)
	}
}

// TestMigrateFlagsLibrarySingles checks 00009 on data: library albums named
// like Apple's singles and EPs get the flag, catalog rows and standard
// albums keep theirs, and the albums domain's next sync is a full one.
func TestMigrateFlagsLibrarySingles(t *testing.T) {
	ctx := context.Background()
	w := openRaw(t, "singles.db")
	p, err := provider(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO albums (library_id, catalog_id, name, updated_at) VALUES
		('l.1', '1', 'Dawn - Single', 1), ('l.2', '2', 'Atlas - EP', 1), ('l.3', '3', 'Dusk', 1), ('l.4', '4', 'dawn - single', 1);
		INSERT INTO albums (catalog_id, name, updated_at) VALUES ('5', 'Cargo - Single', 1);
		INSERT INTO sync_state (domain, last_success, last_full) VALUES ('albums', 9, 9), ('songs', 9, 9)`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 9); err != nil {
		t.Fatal(err)
	}
	rows, err := w.Query(`SELECT name, is_single FROM albums ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rows.Close() })
	var got []string
	for rows.Next() {
		var name string
		var single int
		if err := rows.Scan(&name, &single); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s=%d", name, single))
	}
	if want := "Dawn - Single=1 Atlas - EP=1 Dusk=0 dawn - single=0 Cargo - Single=0"; strings.Join(got, " ") != want {
		t.Fatalf("is_single = %v, want %s", got, want)
	}
	var albums, songs sql.NullInt64
	if err := w.QueryRow(`SELECT (SELECT last_full FROM sync_state WHERE domain = 'albums'), (SELECT last_full FROM sync_state WHERE domain = 'songs')`).Scan(&albums, &songs); err != nil {
		t.Fatal(err)
	}
	if albums.Valid || songs.Int64 != 9 {
		t.Fatalf("last_full = %v (albums), %v (songs)", albums, songs)
	}
}

// TestMigrateBackfillsLibraryRelease checks 00012 on data: the albums and
// songs domains' next sync is a full one, which stores the release fields
// of the library albums and the playParams of the songs (until then an
// album counts as released and a song as playable); other domains and the
// rows themselves keep what they had.
func TestMigrateBackfillsLibraryRelease(t *testing.T) {
	ctx := context.Background()
	w := openRaw(t, "release.db")
	p, err := provider(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 11); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO albums (library_id, catalog_id, name, release_date, updated_at) VALUES ('l.1', '1', 'Soon', '', 1);
		INSERT INTO tracks (library_id, catalog_id, name, album_library_id, updated_at) VALUES ('i.1', '10', 'Not yet', 'l.1', 1);
		INSERT INTO sync_state (domain, last_success, last_full) VALUES ('albums', 9, 9), ('songs', 9, 9), ('playlists', 9, 9)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, w); err != nil {
		t.Fatal(err)
	}
	var pre int
	var catDate string
	var playParams int
	if err := w.QueryRow(`SELECT (SELECT is_prerelease FROM albums), (SELECT catalog_release_date FROM albums), (SELECT no_play_params FROM tracks)`).Scan(&pre, &catDate, &playParams); err != nil {
		t.Fatal(err)
	}
	if pre != 0 || catDate != "" || playParams != 0 {
		t.Fatalf("defaults = %d, %q, %d", pre, catDate, playParams)
	}
	var albums, songs, playlists sql.NullInt64
	if err := w.QueryRow(`SELECT (SELECT last_full FROM sync_state WHERE domain = 'albums'), (SELECT last_full FROM sync_state WHERE domain = 'songs'),
		(SELECT last_full FROM sync_state WHERE domain = 'playlists')`).Scan(&albums, &songs, &playlists); err != nil {
		t.Fatal(err)
	}
	if albums.Valid || songs.Valid || playlists.Int64 != 9 {
		t.Fatalf("last_full = %v (albums), %v (songs), %v (playlists)", albums, songs, playlists)
	}
}

// TestMigrateFoldsCreditNames checks 00022 and 00023 on data: the names
// already stored get their folded key (case, accents), and the songs whose
// credits name an artist that neither a library artist of that name nor a
// catalogue row holds are asked about again (credits_checked 0), no other.
func TestMigrateFoldsCreditNames(t *testing.T) {
	ctx := context.Background()
	w := openRaw(t, "keys.db")
	p, err := provider(w)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 21); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Exec(`INSERT INTO artists (library_id, catalog_id, name, updated_at) VALUES
			('r.az', '432464201', 'Alesso & Zara Larsson', 1), ('r.bey', '5', 'Beyoncé', 1);
		INSERT INTO artists (catalog_id, name, updated_at) VALUES ('7', 'Seven', 1);
		INSERT INTO tracks (library_id, catalog_id, name, artist_name, credits_checked, updated_at) VALUES
			('i.1', '10', 'Words', 'Alesso & Zara Larsson', 100, 1),
			('i.2', '11', 'Halo', 'Beyoncé & Seven', 100, 1);
		INSERT INTO track_artists (track_id, artist_catalog_id, name, position) VALUES
			(1, '432464201', 'Alesso', 0), (1, '5', 'Beyoncé', 1), (2, '5', 'BEYONCE', 0), (2, '7', 'Seven', 1);
		INSERT INTO albums (library_id, catalog_id, name, credits_checked, updated_at) VALUES ('l.1', '20', 'Words - Single', 100, 1);
		INSERT INTO album_artists (album_id, artist_catalog_id, name, position) VALUES (1, '432464201', 'Alesso', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := migrate(ctx, w); err != nil {
		t.Fatal(err)
	}
	keys := func(query string) string {
		rows, err := w.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = rows.Close() }()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		return strings.Join(out, ",")
	}
	if got := keys(`SELECT name_key FROM artists ORDER BY id`); got != "alesso & zara larsson,beyonce,seven" {
		t.Errorf("artist keys = %s", got)
	}
	if got := keys(`SELECT name_key FROM track_artists ORDER BY rowid`); got != "alesso,beyonce,beyonce,seven" {
		t.Errorf("credit keys = %s", got)
	}
	// Song 1 names Alesso, held by neither a library artist of that name nor a
	// catalogue row; song 2 names Beyoncé (a library artist, by name) and Seven
	// (a catalogue row).
	if got := keys(`SELECT credits_checked FROM tracks ORDER BY id`); got != "0,100" {
		t.Errorf("songs' credits_checked = %s, want 0,100", got)
	}
	if got := keys(`SELECT credits_checked FROM albums`); got != "0" {
		t.Errorf("albums' credits_checked = %s, want 0", got)
	}
	// The views key a credit by the artist of that name only.
	if got := keys(`SELECT artist_key FROM track_credit_keys ORDER BY track_id, artist_key`); got != "432464201,r.bey,7,r.bey" {
		t.Errorf("track credit keys = %s, want 432464201,r.bey,7,r.bey", got)
	}
}
