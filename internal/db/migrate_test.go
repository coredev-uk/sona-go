package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// openRaw opens a bare SQLite file for migration tests (no migrations run).
func openRaw(t *testing.T, name string) *sql.DB {
	t.Helper()
	w, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), name)+"?_foreign_keys=1")
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
	if err := migrate(ctx, w); err != nil {
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
