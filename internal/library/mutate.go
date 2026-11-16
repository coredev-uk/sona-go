package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Rated names an item to rate by its catalog resource and/or its library
// copy. Apple keeps one rating for both, so the local mirror stores it
// under each known ID.
type Rated struct {
	// Kind is the catalog type: applemusic.TypeSongs, TypeAlbums,
	// TypeArtists, TypePlaylists or TypeMusicVideos.
	Kind      string
	CatalogID string // "" when the item has no catalog equivalent
	LibraryID string // "" when not in the library (or not known)
}

func (r Rated) keys() []applemusic.ItemKey {
	var out []applemusic.ItemKey
	if r.CatalogID != "" {
		out = append(out, applemusic.ItemKey{Type: r.Kind, ID: r.CatalogID})
	}
	if r.LibraryID != "" {
		out = append(out, applemusic.ItemKey{Type: "library-" + r.Kind, ID: r.LibraryID})
	}
	return out
}

var errNoAPI = errors.New("library: no Apple Music client")

// SetFavorite favourites (on) or un-favourites an item the way the web
// player does: POST/DELETE /v1/me/favorites for catalog items (which also
// adds a favourited song to the library), the rating itself for items with
// no catalog copy (uploads, private playlists). The rating is mirrored
// locally.
func (l *Library) SetFavorite(ctx context.Context, it Rated, on bool) error {
	if l.api == nil {
		return errNoAPI
	}
	var err error
	switch {
	case it.CatalogID != "" && on:
		err = l.api.Favorite(ctx, it.Kind, []string{it.CatalogID})
	case it.CatalogID != "":
		err = l.api.Unfavorite(ctx, it.Kind, []string{it.CatalogID})
	case it.LibraryID != "" && on:
		err = l.api.SetRating(ctx, "library-"+it.Kind, it.LibraryID, applemusic.RatingFavorite)
	case it.LibraryID != "":
		err = l.api.DeleteRating(ctx, "library-"+it.Kind, it.LibraryID)
	default:
		return fmt.Errorf("library: favourite: no ID for %s", it.Kind)
	}
	if err != nil {
		return err
	}
	return l.MirrorRating(ctx, it, ratingValue(on, applemusic.RatingFavorite))
}

// SetSuggestLess sets (on) or clears an item's Suggest Less rating on Apple
// and mirrors it locally.
func (l *Library) SetSuggestLess(ctx context.Context, it Rated, on bool) error {
	if l.api == nil {
		return errNoAPI
	}
	keys := it.keys()
	if len(keys) == 0 {
		return fmt.Errorf("library: suggest less: no ID for %s", it.Kind)
	}
	k := keys[0] // the catalog resource when there is one
	var err error
	if on {
		err = l.api.SetRating(ctx, k.Type, k.ID, applemusic.RatingSuggestLess)
	} else {
		err = l.api.DeleteRating(ctx, k.Type, k.ID)
	}
	if err != nil {
		return err
	}
	return l.MirrorRating(ctx, it, ratingValue(on, applemusic.RatingSuggestLess))
}

// ratingValue is the rating v when on, else 0 (no rating).
func ratingValue(on bool, v int) int {
	if on {
		return v
	}
	return 0
}

// MirrorRating stores value (0 clears) as the local rating of every ID of
// it, without calling Apple.
func (l *Library) MirrorRating(ctx context.Context, it Rated, value int) error {
	now := db.Millis(time.Now())
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		for _, k := range it.keys() {
			var err error
			if value == 0 {
				err = q.DeleteFavorite(ctx, dbq.DeleteFavoriteParams{Kind: k.Type, AppleID: k.ID})
			} else {
				err = q.UpsertFavorite(ctx, dbq.UpsertFavoriteParams{Kind: k.Type, AppleID: k.ID, Value: int64(value), UpdatedAt: now})
			}
			if err != nil {
				return fmt.Errorf("library: mirror rating: %w", err)
			}
		}
		return nil
	})
}

// Library item kinds (library_items.kind) by catalog type.
var itemKinds = map[string]string{
	applemusic.TypeSongs:     "song",
	applemusic.TypeAlbums:    "album",
	applemusic.TypeArtists:   "artist",
	applemusic.TypePlaylists: "playlist",
}

// Remove deletes library item libraryID of catalog type kind
// (applemusic.TypeSongs, TypeAlbums, TypeArtists, TypePlaylists,
// TypeMusicVideos) from the user's library. The local store forgets it at
// once (see Hide), before Apple answers, so lists and counts are right
// without waiting; changed (nil for none) is called then, and again when
// Apple refuses the delete and the item is put back. After Apple's
// acceptance the rows nothing references any more are dropped (see Forget).
func (l *Library) Remove(ctx context.Context, kind, libraryID string, changed func()) error {
	if l.api == nil {
		return errNoAPI
	}
	if kind == applemusic.TypeMusicVideos {
		// Apple's answer first: a music video has no ratings, playlists or
		// search entry to put back, so there is nothing to hide meanwhile.
		if err := l.api.DeleteFromLibrary(ctx, kind, libraryID); err != nil {
			return err
		}
		if err := l.db.Q.DeleteLibraryMusicVideo(ctx, libraryID); err != nil {
			return fmt.Errorf("library: forget music video: %w", err)
		}
		if changed != nil {
			changed()
		}
		return nil
	}
	if _, ok := itemKinds[kind]; !ok {
		return l.api.DeleteFromLibrary(ctx, kind, libraryID)
	}
	hidden, err := l.Hide(ctx, kind, libraryID)
	if err != nil {
		return err
	}
	if changed != nil {
		changed()
	}
	if err := l.api.DeleteFromLibrary(ctx, kind, libraryID); err != nil {
		if rerr := l.Restore(context.WithoutCancel(ctx), hidden); rerr != nil {
			err = errors.Join(err, rerr)
		}
		if changed != nil {
			changed()
		}
		return err
	}
	return l.Forget(ctx, kind, libraryID)
}

// forgetMembers select, per removed kind, the library items Apple removes
// with it: an album takes its songs, an artist its albums and songs.
var forgetMembers = map[string][]struct{ kind, query string }{
	"song": {
		{"song", `SELECT li.ref, t.library_id FROM library_items li JOIN tracks t ON t.id = li.ref
			WHERE li.kind = 'song' AND t.library_id = ?`},
	},
	"album": {
		{"album", `SELECT li.ref, a.library_id FROM library_items li JOIN albums a ON a.id = li.ref
			WHERE li.kind = 'album' AND a.library_id = ?`},
		{"song", `SELECT li.ref, t.library_id FROM library_items li JOIN tracks t ON t.id = li.ref
			WHERE li.kind = 'song' AND t.album_library_id = ?`},
	},
	"artist": {
		{"artist", `SELECT li.ref, r.library_id FROM library_items li JOIN artists r ON r.id = li.ref
			WHERE li.kind = 'artist' AND r.library_id = ?`},
		{"album", `SELECT li.ref, a.library_id FROM library_items li JOIN albums a ON a.id = li.ref
			WHERE li.kind = 'album' AND a.artist_library_id = ?`},
		{"song", `SELECT li.ref, t.library_id FROM library_items li JOIN tracks t ON t.id = li.ref
			WHERE li.kind = 'song' AND t.artist_library_id = ?`},
	},
	"playlist": {
		{"playlist", `SELECT li.ref, p.library_id FROM library_items li JOIN playlists p ON p.id = li.ref
			WHERE li.kind = 'playlist' AND p.library_id = ?`},
	},
}

// Forget removes library item libraryID of catalog type kind from the
// local store only: its library membership (and that of the songs and
// albums Apple removes with it, and of the albums and artists left with no
// library songs), their ratings and search entries, and removed songs from
// playlists. Rows nothing references any more are dropped. Forgetting an
// unknown ID does nothing.
func (l *Library) Forget(ctx context.Context, kind, libraryID string) error {
	tx, err := l.db.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("library: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := forget(ctx, tx, kind, libraryID, nil); err != nil {
		return err
	}
	if err := gc(ctx, l.db.Q.WithTx(tx)); err != nil {
		return fmt.Errorf("library: forget: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("library: commit: %w", err)
	}
	return nil
}

// Hidden is what Hide took out of the local library, to put back with
// Restore.
type Hidden struct {
	items     []hiddenItem
	favorites []hiddenFavorite
	search    []hiddenSearch
	tracks    []hiddenPlaylistTrack
}

type hiddenItem struct {
	kind                 string
	ref, generation      int64
	dateAdded, addedRank sql.NullInt64
}

type hiddenFavorite struct {
	kind, id         string
	value, updatedAt int64
}

type hiddenSearch struct {
	kind, ref, artwork, name, artist, album string
}

type hiddenPlaylistTrack struct{ playlist, position, track int64 }

// Hide removes what Forget removes, except that rows nothing references any
// more stay (so Restore can put the item back), and returns what it took
// out. It is the local half of Remove.
func (l *Library) Hide(ctx context.Context, kind, libraryID string) (*Hidden, error) {
	tx, err := l.db.W.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("library: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	h := &Hidden{}
	if err := forget(ctx, tx, kind, libraryID, h); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("library: commit: %w", err)
	}
	return h, nil
}

// Restore puts back what Hide took out: the library membership, ratings,
// search entries and playlist places of the items. An item whose row a sync
// dropped meanwhile stays out (the next sync brings it back if Apple still
// lists it).
func (l *Library) Restore(ctx context.Context, h *Hidden) error {
	if h == nil {
		return nil
	}
	tx, err := l.db.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("library: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	exec := func(q string, args ...any) (int64, error) {
		res, err := tx.ExecContext(ctx, q, args...)
		if err != nil {
			return 0, fmt.Errorf("library: restore: %w", err)
		}
		n, _ := res.RowsAffected()
		return n, nil
	}
	for _, it := range h.items {
		// The table name is one of itemTables' constants.
		if _, err := exec(`INSERT OR IGNORE INTO library_items (kind, ref, date_added, added_rank, generation)
			SELECT ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM `+itemTables[it.kind]+` WHERE id = ?)`,
			it.kind, it.ref, it.dateAdded, it.addedRank, it.generation, it.ref); err != nil {
			return err
		}
	}
	for _, f := range h.favorites {
		if _, err := exec(`INSERT OR REPLACE INTO favorites (kind, apple_id, value, updated_at) VALUES (?, ?, ?, ?)`,
			f.kind, f.id, f.value, f.updatedAt); err != nil {
			return err
		}
	}
	for _, e := range h.search {
		if _, err := exec(`INSERT INTO search_fts (kind, ref, artwork, name, artist, album) VALUES (?, ?, ?, ?, ?, ?)`,
			e.kind, e.ref, e.artwork, e.name, e.artist, e.album); err != nil {
			return err
		}
	}
	for _, pt := range h.tracks {
		n, err := exec(`INSERT OR IGNORE INTO playlist_tracks (playlist_id, position, track_id)
			SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM tracks WHERE id = ?) AND EXISTS (SELECT 1 FROM playlists WHERE id = ?)`,
			pt.playlist, pt.position, pt.track, pt.track, pt.playlist)
		if err != nil {
			return err
		}
		if n > 0 {
			if _, err := exec(`UPDATE playlists SET track_count = track_count + 1 WHERE id = ?`, pt.playlist); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("library: commit: %w", err)
	}
	return nil
}

// itemTables names the table a library item kind's ref points into.
var itemTables = map[string]string{"song": "tracks", "album": "albums", "artist": "artists", "playlist": "playlists"}

// forgotten is a library item forget removes: its kind, row and Apple ID.
type forgotten struct {
	kind string
	ref  int64
	id   string
}

// forget is Forget's and Hide's work in tx, without the sweep of unreferenced
// rows; when h is not nil, it records what it removes.
func forget(ctx context.Context, tx *sql.Tx, kind, libraryID string, h *Hidden) error {
	members, ok := forgetMembers[itemKinds[kind]]
	if !ok {
		return fmt.Errorf("library: forget %s %s: unsupported kind", kind, libraryID)
	}
	var first []forgotten
	for _, m := range members {
		rows, err := tx.QueryContext(ctx, m.query, libraryID)
		if err != nil {
			return fmt.Errorf("library: forget: %w", err)
		}
		for rows.Next() {
			it := forgotten{kind: m.kind}
			var id sql.NullString
			if err := rows.Scan(&it.ref, &id); err != nil {
				_ = rows.Close()
				return fmt.Errorf("library: forget: %w", err)
			}
			it.id = id.String
			first = append(first, it)
		}
		_ = rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("library: forget: %w", err)
		}
	}
	if err := drop(ctx, tx, first, h); err != nil {
		return err
	}
	// What the removal left empty: albums with no library songs, then
	// artists with no library albums or songs.
	albums, err := emptyAlbums(ctx, tx, first)
	if err != nil {
		return err
	}
	if err := drop(ctx, tx, albums, h); err != nil {
		return err
	}
	artists, err := emptyArtists(ctx, tx, append(first, albums...))
	if err != nil {
		return err
	}
	return drop(ctx, tx, artists, h)
}

// drop removes the items' library membership, ratings and search entries
// (and removed songs from playlists), recording them in h when it is not
// nil.
func drop(ctx context.Context, tx *sql.Tx, items []forgotten, h *Hidden) error {
	for _, it := range items {
		if h != nil {
			if err := h.record(ctx, tx, it); err != nil {
				return err
			}
		}
		stmts := []struct {
			q    string
			args []any
		}{
			{`DELETE FROM library_items WHERE kind = ? AND ref = ?`, []any{it.kind, it.ref}},
			{`DELETE FROM favorites WHERE kind = ? AND apple_id = ?`, []any{"library-" + it.kind + "s", it.id}},
			{`DELETE FROM search_fts WHERE kind = ? AND ref = ?`, []any{it.kind, it.id}},
		}
		if it.kind == "song" {
			// Apple also takes a deleted song out of every playlist.
			stmts = append(stmts,
				struct {
					q    string
					args []any
				}{`UPDATE playlists SET track_count = track_count -
					(SELECT count(*) FROM playlist_tracks pt WHERE pt.playlist_id = playlists.id AND pt.track_id = ?)
					WHERE id IN (SELECT playlist_id FROM playlist_tracks WHERE track_id = ?)`, []any{it.ref, it.ref}},
				struct {
					q    string
					args []any
				}{`DELETE FROM playlist_tracks WHERE track_id = ?`, []any{it.ref}},
			)
		}
		for _, s := range stmts {
			if _, err := tx.ExecContext(ctx, s.q, s.args...); err != nil {
				return fmt.Errorf("library: forget %s %s: %w", it.kind, it.id, err)
			}
		}
	}
	return nil
}

// record notes what drop is about to remove of it.
func (h *Hidden) record(ctx context.Context, tx *sql.Tx, it forgotten) error {
	var li hiddenItem
	switch err := tx.QueryRowContext(ctx, `SELECT kind, ref, generation, date_added, added_rank FROM library_items WHERE kind = ? AND ref = ?`,
		it.kind, it.ref).Scan(&li.kind, &li.ref, &li.generation, &li.dateAdded, &li.addedRank); {
	case err == nil:
		h.items = append(h.items, li)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("library: forget: %w", err)
	}
	f := hiddenFavorite{kind: "library-" + it.kind + "s", id: it.id}
	switch err := tx.QueryRowContext(ctx, `SELECT value, updated_at FROM favorites WHERE kind = ? AND apple_id = ?`, f.kind, f.id).Scan(&f.value, &f.updatedAt); {
	case err == nil:
		h.favorites = append(h.favorites, f)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("library: forget: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT kind, ref, artwork, name, artist, album FROM search_fts WHERE kind = ? AND ref = ?`, it.kind, it.id)
	if err != nil {
		return fmt.Errorf("library: forget: %w", err)
	}
	for rows.Next() {
		var e hiddenSearch
		if err := rows.Scan(&e.kind, &e.ref, &e.artwork, &e.name, &e.artist, &e.album); err != nil {
			_ = rows.Close()
			return fmt.Errorf("library: forget: %w", err)
		}
		h.search = append(h.search, e)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("library: forget: %w", err)
	}
	if it.kind != "song" {
		return nil
	}
	rows, err = tx.QueryContext(ctx, `SELECT playlist_id, position, track_id FROM playlist_tracks WHERE track_id = ?`, it.ref)
	if err != nil {
		return fmt.Errorf("library: forget: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pt hiddenPlaylistTrack
		if err := rows.Scan(&pt.playlist, &pt.position, &pt.track); err != nil {
			return fmt.Errorf("library: forget: %w", err)
		}
		h.tracks = append(h.tracks, pt)
	}
	return rows.Err()
}

// owners are the Apple IDs, by q (which selects a row's one ID column for a
// row id), of what the removed items of kind belong to.
func owners(ctx context.Context, tx *sql.Tx, removed []forgotten, kind, q string) ([]string, error) {
	var out []string
	for _, it := range removed {
		if it.kind != kind {
			continue
		}
		var id sql.NullString
		if err := tx.QueryRowContext(ctx, q, it.ref).Scan(&id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("library: forget: %w", err)
		}
		if id.Valid && !slices.Contains(out, id.String) {
			out = append(out, id.String)
		}
	}
	return out, nil
}

// emptyAlbums finds the library albums of the removed songs that hold no
// library song any more. Pre-added albums stay: they may hold none yet.
func emptyAlbums(ctx context.Context, tx *sql.Tx, removed []forgotten) ([]forgotten, error) {
	ids, err := owners(ctx, tx, removed, "song", `SELECT album_library_id FROM tracks WHERE id = ?`)
	if err != nil {
		return nil, err
	}
	var out []forgotten
	for _, id := range ids {
		a := forgotten{kind: "album", id: id}
		err := tx.QueryRowContext(ctx, `SELECT a.id FROM albums a
			JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
			WHERE a.library_id = ? AND a.is_prerelease = 0
			AND NOT EXISTS (SELECT 1 FROM tracks t JOIN library_items ls ON ls.kind = 'song' AND ls.ref = t.id
				WHERE t.album_library_id = a.library_id)`, id).Scan(&a.ref)
		switch {
		case err == nil:
			out = append(out, a)
		case !errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("library: forget: %w", err)
		}
	}
	return out, nil
}

// emptyArtists finds the library artists of the removed songs and albums
// that have no library album or song any more.
func emptyArtists(ctx context.Context, tx *sql.Tx, removed []forgotten) ([]forgotten, error) {
	ids, err := owners(ctx, tx, removed, "song", `SELECT artist_library_id FROM tracks WHERE id = ?`)
	if err != nil {
		return nil, err
	}
	more, err := owners(ctx, tx, removed, "album", `SELECT artist_library_id FROM albums WHERE id = ?`)
	if err != nil {
		return nil, err
	}
	for _, id := range more {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	var out []forgotten
	for _, id := range ids {
		r := forgotten{kind: "artist", id: id}
		err := tx.QueryRowContext(ctx, `SELECT r.id FROM artists r
			JOIN library_items li ON li.kind = 'artist' AND li.ref = r.id
			WHERE r.library_id = ?
			AND NOT EXISTS (SELECT 1 FROM albums a JOIN library_items la ON la.kind = 'album' AND la.ref = a.id
				WHERE a.artist_library_id = r.library_id)
			AND NOT EXISTS (SELECT 1 FROM tracks t JOIN library_items ls ON ls.kind = 'song' AND ls.ref = t.id
				WHERE t.artist_library_id = r.library_id)`, id).Scan(&r.ref)
		switch {
		case err == nil:
			out = append(out, r)
		case !errors.Is(err, sql.ErrNoRows):
			return nil, fmt.Errorf("library: forget: %w", err)
		}
	}
	return out, nil
}

// pruneEmpty forgets the library albums that hold no library song (pre-added
// albums, which may hold none yet, stay), whatever removed their songs:
// Apple may keep listing an album after its last song is deleted, and Sona
// shows no empty album. Artists stay: Apple lists them on its own, and an
// artist with nothing is a library artist all the same. It reports whether
// it removed anything.
func pruneEmpty(ctx context.Context, d *db.DB) (bool, error) {
	tx, err := d.W.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("library: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT a.id, a.library_id FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.is_prerelease = 0
		AND NOT EXISTS (SELECT 1 FROM tracks t JOIN library_items ls ON ls.kind = 'song' AND ls.ref = t.id
			WHERE t.album_library_id = a.library_id)`)
	if err != nil {
		return false, fmt.Errorf("library: prune empty: %w", err)
	}
	var items []forgotten
	for rows.Next() {
		it := forgotten{kind: "album"}
		var id sql.NullString
		if err := rows.Scan(&it.ref, &id); err != nil {
			_ = rows.Close()
			return false, fmt.Errorf("library: prune empty: %w", err)
		}
		it.id = id.String
		items = append(items, it)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("library: prune empty: %w", err)
	}
	if len(items) == 0 {
		return false, nil
	}
	if err := drop(ctx, tx, items, nil); err != nil {
		return false, err
	}
	if err := gc(ctx, d.Q.WithTx(tx)); err != nil {
		return false, fmt.Errorf("library: prune empty: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("library: commit: %w", err)
	}
	return true, nil
}
