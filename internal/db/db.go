// Package db opens Sona's SQLite store (github.com/mattn/go-sqlite3, FTS5
// via the sqlite_fts5 build tag), applies the embedded goose migrations and
// exposes the sqlc queries in package dbq.
//
// The schema is plain SQLite so it stays libSQL-compatible. Timestamps are
// Unix milliseconds (see Millis and Time).
package db

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"runtime"
	"slices"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // driver "sqlite3"
	"github.com/pressly/goose/v3"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB is the store: one writer connection (SQLite allows a single writer;
// serialising in Go avoids SQLITE_BUSY) and a pool of read-only connections
// that run concurrently with it under WAL.
type DB struct {
	W  *sql.DB      // writer, max one connection; transactions BEGIN IMMEDIATE
	R  *sql.DB      // read-only pool
	Q  *dbq.Queries // queries on W
	RQ *dbq.Queries // queries on R
}

// Open opens (creating if needed) the database file at path, enables WAL,
// foreign keys and a busy timeout, and migrates it to the latest schema.
// path must be a file; in-memory databases are not supported because the
// writer and the read pool must share it.
func Open(ctx context.Context, path string) (*DB, error) {
	if path == "" || path == ":memory:" {
		return nil, errors.New("db: a file path is required")
	}
	base := "file:" + (&url.URL{Path: path}).EscapedPath() + "?_foreign_keys=1&_busy_timeout=5000&_cache_size=-16000"
	w, err := sql.Open("sqlite3", base+"&_journal_mode=WAL&_synchronous=NORMAL&_txlock=immediate")
	if err != nil {
		return nil, fmt.Errorf("db: open writer: %w", err)
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)
	w.SetConnMaxLifetime(0)
	w.SetConnMaxIdleTime(0)
	if err := w.PingContext(ctx); err != nil {
		w.Close()
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}
	if err := migrate(ctx, w); err != nil {
		w.Close()
		return nil, err
	}
	r, err := sql.Open("sqlite3", base+"&_query_only=1")
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("db: open readers: %w", err)
	}
	n := max(4, runtime.NumCPU())
	r.SetMaxOpenConns(n)
	r.SetMaxIdleConns(n)
	return &DB{W: w, R: r, Q: dbq.New(w), RQ: dbq.New(r)}, nil
}

func migrate(ctx context.Context, w *sql.DB) error {
	p, err := provider(w)
	if err != nil {
		return err
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	return nil
}

// provider returns the goose provider over the embedded migrations. They are
// numbered densely (00001, 00002, …) and applied strictly in order.
func provider(w *sql.DB) (*goose.Provider, error) {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, w, fsys)
	if err != nil {
		return nil, fmt.Errorf("db: migrations: %w", err)
	}
	return p, nil
}

// Close closes both pools.
func (d *DB) Close() error {
	return errors.Join(d.R.Close(), d.W.Close())
}

// Tx runs fn in a write transaction and commits if fn returns nil.
func (d *DB) Tx(ctx context.Context, fn func(q *dbq.Queries) error) error {
	tx, err := d.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	if err := fn(d.Q.WithTx(tx)); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	return nil
}

const wipeUserData = `
DELETE FROM library_items;
DELETE FROM playlist_tracks;
DELETE FROM playlists WHERE library_id IS NOT NULL;
DELETE FROM tracks WHERE library_id IS NOT NULL;
DELETE FROM albums WHERE library_id IS NOT NULL;
DELETE FROM artists WHERE library_id IS NOT NULL;
DELETE FROM playback_history;
DELETE FROM favorites;
DELETE FROM pins;
DELETE FROM playlist_folders;
DELETE FROM sync_state;
DELETE FROM accounts;
DELETE FROM search_fts;
DELETE FROM recent_searches;
DELETE FROM release_reminders;
DELETE FROM discover_cache WHERE personal = 1;
`

// WipeUserData deletes everything tied to the signed-in user (library rows
// and membership, playlists, history, ratings, pins, sync checkpoints, the
// account row, the search index, recent searches and release reminders)
// for sign-out. Catalog rows and the catalog response cache are storefront
// data, not user data, and are kept.
func (d *DB) WipeUserData(ctx context.Context) error {
	tx, err := d.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	if _, err := tx.ExecContext(ctx, wipeUserData); err != nil {
		tx.Rollback()
		return fmt.Errorf("db: wipe user data: %w", err)
	}
	return tx.Commit()
}

// userArtwork lists the artwork references (templates, or Go-served /art/
// URLs for recent searches) held by user data.
const userArtwork = `
SELECT artwork_template FROM playlists WHERE library_id IS NOT NULL
UNION SELECT artwork_template FROM tracks WHERE library_id IS NOT NULL
UNION SELECT artwork_template FROM albums WHERE library_id IS NOT NULL
UNION SELECT artwork_template FROM artists WHERE library_id IS NOT NULL
UNION SELECT artwork_template FROM pins
UNION SELECT artwork_template FROM playback_history
UNION SELECT art_url FROM recent_searches
UNION SELECT artwork_template FROM release_reminders
`

// catalogArtwork lists the artwork templates of catalog rows.
const catalogArtwork = `
SELECT artwork_template FROM playlists WHERE library_id IS NULL
UNION SELECT artwork_template FROM tracks WHERE library_id IS NULL
UNION SELECT artwork_template FROM albums WHERE library_id IS NULL
UNION SELECT artwork_template FROM artists WHERE library_id IS NULL
`

// UserArtwork returns the artwork that only the signed-in user's data
// references, for sign-out to remove from the artwork cache before
// WipeUserData: library rows (playlist covers above all), history, pins,
// recent searches and the personal Home page. Artwork that catalog rows
// also use is storefront data and is left out.
func (d *DB) UserArtwork(ctx context.Context) ([]string, error) {
	refs := map[string]bool{}
	add := func(query string, fn func(string)) error {
		rows, err := d.R.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("db: user artwork: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return fmt.Errorf("db: user artwork: %w", err)
			}
			fn(s)
		}
		return rows.Err()
	}
	if err := add(userArtwork, func(s string) { refs[s] = s != "" }); err != nil {
		return nil, err
	}
	// Personal Home bodies are Apple's JSON: every artwork template in them.
	if err := add(`SELECT CAST(body AS TEXT) FROM discover_cache WHERE personal = 1`, func(body string) {
		var v any
		if json.Unmarshal([]byte(body), &v) == nil {
			templatesIn(v, refs)
		}
	}); err != nil {
		return nil, err
	}
	if err := add(catalogArtwork, func(s string) { delete(refs, s) }); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(refs))
	for r, ok := range refs {
		if ok {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out, nil
}

// templatesIn adds every artwork template ("…{w}x{h}…") in a decoded JSON
// value to refs.
func templatesIn(v any, refs map[string]bool) {
	switch v := v.(type) {
	case string:
		if strings.Contains(v, "{w}") {
			refs[v] = true
		}
	case []any:
		for _, e := range v {
			templatesIn(e, refs)
		}
	case map[string]any:
		for _, e := range v {
			templatesIn(e, refs)
		}
	}
}

// Maintenance thresholds: VACUUM runs when free pages hold at least
// vacuumMinFree bytes and vacuumMinRatio of the file.
const (
	vacuumMinFree  = 4 << 20
	vacuumMinRatio = 0.1
)

// Space reports the main database file's size and the bytes its free
// pages hold (reclaimable by VACUUM).
func (d *DB) Space(ctx context.Context) (size, free int64, err error) {
	var pages, freePages, pageSize int64
	err = d.R.QueryRowContext(ctx, `SELECT (SELECT page_count FROM pragma_page_count()), (SELECT freelist_count FROM pragma_freelist_count()), (SELECT page_size FROM pragma_page_size())`).
		Scan(&pages, &freePages, &pageSize)
	if err != nil {
		return 0, 0, fmt.Errorf("db: space: %w", err)
	}
	return pages * pageSize, freePages * pageSize, nil
}

// Maintain runs SQLite upkeep on the writer: PRAGMA optimize, and VACUUM
// (then a WAL truncate) when force is set or enough of the file is free
// pages. It holds the writer for the duration, so callers run it when the
// app is idle. It returns the bytes VACUUM gave back.
func (d *DB) Maintain(ctx context.Context, force bool) (int64, error) {
	if _, err := d.W.ExecContext(ctx, `PRAGMA optimize`); err != nil {
		return 0, fmt.Errorf("db: optimize: %w", err)
	}
	size, free, err := d.Space(ctx)
	if err != nil {
		return 0, err
	}
	if !force && (free < vacuumMinFree || float64(free) < vacuumMinRatio*float64(size)) {
		return 0, nil
	}
	if _, err := d.W.ExecContext(ctx, `VACUUM`); err != nil {
		return 0, fmt.Errorf("db: vacuum: %w", err)
	}
	if _, err := d.W.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return 0, fmt.Errorf("db: checkpoint: %w", err)
	}
	after, _, err := d.Space(ctx)
	if err != nil {
		return 0, err
	}
	return max(size-after, 0), nil
}

// Millis converts t to the Unix-millisecond form stored in the schema.
func Millis(t time.Time) int64 { return t.UnixMilli() }

// Time converts a stored Unix-millisecond value back to a time.Time.
func Time(ms int64) time.Time { return time.UnixMilli(ms) }

// NullString maps "" to SQL NULL.
func NullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// NullMillis maps the zero time to SQL NULL.
func NullMillis(t time.Time) sql.NullInt64 {
	if t.IsZero() {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

// Bool maps a Go bool to SQLite's 0/1.
func Bool(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
