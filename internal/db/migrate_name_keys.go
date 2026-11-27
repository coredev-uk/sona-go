package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/coredev-uk/sona-go/internal/namefold"
)

// nameKeysVersion is the version of the migration that fills the name_key
// columns 00022 added. It is a Go migration because the folding is Unicode's
// (internal/namefold), which SQLite cannot do.
const nameKeysVersion = 23

// nameKeysMigration (00023) folds the names already stored (artists and the
// credits of songs and albums) into their name_key, then has the catalogue
// asked again about the songs and albums whose credits name an artist that has
// neither a library artist of that name nor a catalogue row (a credit-only
// artist, listed with no picture): the credits pass now stores the artists
// from the answer it already gets. No other credit is asked again.
func nameKeysMigration() *goose.Migration {
	return goose.NewGoMigration(nameKeysVersion, &goose.GoFunc{RunTx: upNameKeys}, nil)
}

func upNameKeys(ctx context.Context, tx *sql.Tx) error {
	for _, table := range []string{"artists", "track_artists", "album_artists"} {
		if err := foldNames(ctx, tx, table); err != nil {
			return err
		}
	}
	for _, item := range []struct{ table, credits, ref string }{
		{"tracks", "track_artists", "track_id"},
		{"albums", "album_artists", "album_id"},
	} {
		_, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %[1]s SET credits_checked = 0 WHERE id IN (
			SELECT c.%[3]s FROM %[2]s c
			WHERE NOT EXISTS (SELECT 1 FROM artists x WHERE x.library_id IS NULL AND x.catalog_id = c.artist_catalog_id)
			  AND NOT EXISTS (SELECT 1 FROM artists r WHERE r.library_id IS NOT NULL
			                  AND r.catalog_id = c.artist_catalog_id AND r.name_key = c.name_key))`,
			item.table, item.credits, item.ref))
		if err != nil {
			return fmt.Errorf("db: ask %s credits again: %w", item.table, err)
		}
	}
	return nil
}

// foldNames sets name_key of every row of table to its name's fold. Rows are
// read first: a statement cannot write the table a cursor is reading.
func foldNames(ctx context.Context, tx *sql.Tx, table string) error {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`SELECT rowid, name FROM %s`, table))
	if err != nil {
		return fmt.Errorf("db: read %s names: %w", table, err)
	}
	type row struct {
		id        int64
		name, key string
	}
	var all []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name); err != nil {
			_ = rows.Close()
			return fmt.Errorf("db: read %s names: %w", table, err)
		}
		r.key = namefold.Fold(r.name)
		all = append(all, r)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("db: read %s names: %w", table, err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("db: read %s names: %w", table, err)
	}
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`UPDATE %s SET name_key = ? WHERE rowid = ?`, table))
	if err != nil {
		return fmt.Errorf("db: fold %s names: %w", table, err)
	}
	defer func() { _ = stmt.Close() }()
	for _, r := range all {
		if _, err := stmt.ExecContext(ctx, r.key, r.id); err != nil {
			return fmt.Errorf("db: fold %s names: %w", table, err)
		}
	}
	return nil
}
