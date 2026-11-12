package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
// TypeMusicVideos) from the user's library on Apple, then from the local
// store at once (see Forget), so lists and counts are right before the next
// sync.
func (l *Library) Remove(ctx context.Context, kind, libraryID string) error {
	if l.api == nil {
		return errNoAPI
	}
	if err := l.api.DeleteFromLibrary(ctx, kind, libraryID); err != nil {
		return err
	}
	if _, ok := itemKinds[kind]; !ok {
		return nil // music videos are not stored locally
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
// albums Apple removes with it), their ratings and search entries, and
// removed songs from playlists. Rows nothing references any more are
// dropped. Forgetting an unknown ID does nothing.
func (l *Library) Forget(ctx context.Context, kind, libraryID string) error {
	members, ok := forgetMembers[itemKinds[kind]]
	if !ok {
		return fmt.Errorf("library: forget %s %s: unsupported kind", kind, libraryID)
	}
	tx, err := l.db.W.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("library: begin: %w", err)
	}
	defer tx.Rollback()
	type item struct {
		ref int64
		id  string
	}
	removed := map[string][]item{}
	for _, m := range members {
		rows, err := tx.QueryContext(ctx, m.query, libraryID)
		if err != nil {
			return fmt.Errorf("library: forget: %w", err)
		}
		for rows.Next() {
			var it item
			var id sql.NullString
			if err := rows.Scan(&it.ref, &id); err != nil {
				rows.Close()
				return fmt.Errorf("library: forget: %w", err)
			}
			it.id = id.String
			removed[m.kind] = append(removed[m.kind], it)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return fmt.Errorf("library: forget: %w", err)
		}
	}
	type stmt struct {
		q    string
		args []any
	}
	for kind, items := range removed {
		for _, it := range items {
			stmts := []stmt{
				{`DELETE FROM library_items WHERE kind = ? AND ref = ?`, []any{kind, it.ref}},
				{`DELETE FROM favorites WHERE kind = ? AND apple_id = ?`, []any{"library-" + kind + "s", it.id}},
				{`DELETE FROM search_fts WHERE kind = ? AND ref = ?`, []any{kind, it.id}},
			}
			if kind == "song" {
				// Apple also takes a deleted song out of every playlist.
				stmts = append(stmts,
					stmt{`UPDATE playlists SET track_count = track_count -
						(SELECT count(*) FROM playlist_tracks pt WHERE pt.playlist_id = playlists.id AND pt.track_id = ?)
						WHERE id IN (SELECT playlist_id FROM playlist_tracks WHERE track_id = ?)`, []any{it.ref, it.ref}},
					stmt{`DELETE FROM playlist_tracks WHERE track_id = ?`, []any{it.ref}},
				)
			}
			for _, s := range stmts {
				if _, err := tx.ExecContext(ctx, s.q, s.args...); err != nil {
					return fmt.Errorf("library: forget %s %s: %w", kind, it.id, err)
				}
			}
		}
	}
	if err := gc(ctx, l.db.Q.WithTx(tx)); err != nil {
		return fmt.Errorf("library: forget: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("library: commit: %w", err)
	}
	return nil
}
