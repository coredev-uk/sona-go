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
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"runtime"
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
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	// Feature migrations are numbered per slice and can land in any order,
	// so a database that already has a later one still gets an earlier one.
	p, err := goose.NewProvider(goose.DialectSQLite3, w, fsys, goose.WithAllowOutofOrder(true))
	if err != nil {
		return fmt.Errorf("db: migrations: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	return nil
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
DELETE FROM discover_cache WHERE personal = 1;
`

// WipeUserData deletes everything tied to the signed-in user (library rows
// and membership, playlists, history, ratings, pins, sync checkpoints, the
// account row and the search index) for sign-out. Catalog rows and the
// catalog response cache are storefront data, not user data, and are kept.
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
