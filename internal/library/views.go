package library

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Added is one Recently Added entry: a library album or playlist (the
// other is nil), newest first.
type Added struct {
	Album    *Album
	Playlist *Playlist
}

// recentlyAddedFrom selects the library albums and playlists: library
// rows whose album or playlist row exists. The page and its total both go
// through it, so a row that lost its target is neither listed nor counted.
const recentlyAddedFrom = ` FROM library_items li
	LEFT JOIN albums a ON li.kind = 'album' AND a.id = li.ref
	LEFT JOIN playlists p ON li.kind = 'playlist' AND p.id = li.ref
	WHERE li.kind IN ('album', 'playlist') AND coalesce(a.library_id, p.library_id) IS NOT NULL`

// RecentlyAdded lists library albums and playlists by the date they were
// added, newest first, the way Apple's Recently Added does (songs are
// listed through their albums). It also returns the size of the whole
// list.
func (l *Library) RecentlyAdded(ctx context.Context, o ListOptions) ([]Added, int, error) {
	limit, offset := o.page()
	type ref struct{ kind, id string }
	refs, err := query(ctx, l, `SELECT li.kind, coalesce(a.library_id, p.library_id)`+recentlyAddedFrom+`
		ORDER BY li.date_added IS NULL, li.date_added DESC, li.kind, li.ref DESC
		LIMIT ? OFFSET ?`, func(sc scanner) (ref, error) {
		var r ref
		err := sc.Scan(&r.kind, &r.id)
		return r, err
	}, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := l.db.R.QueryRowContext(ctx, `SELECT count(*)`+recentlyAddedFrom).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("library: recently added: %w", err)
	}
	var albumIDs, playlistIDs []string
	for _, r := range refs {
		if r.kind == "album" {
			albumIDs = append(albumIDs, r.id)
		} else {
			playlistIDs = append(playlistIDs, r.id)
		}
	}
	albums, err := byIDs(ctx, l, albumsByID, albumIDs, scanAlbum, func(a Album) string { return a.LibraryID })
	if err != nil {
		return nil, 0, err
	}
	pls, err := byIDs(ctx, l, playlistsByID, playlistIDs, scanPlaylist, func(p Playlist) string { return p.LibraryID })
	if err == nil {
		err = l.FillCovers(ctx, pls)
	}
	if err != nil {
		return nil, 0, err
	}
	byID := make(map[string]Added, len(refs))
	for i := range albums {
		byID["album:"+albums[i].LibraryID] = Added{Album: &albums[i]}
	}
	for i := range pls {
		byID["playlist:"+pls[i].LibraryID] = Added{Playlist: &pls[i]}
	}
	out := make([]Added, 0, len(refs))
	for _, r := range refs {
		if a, ok := byID[r.kind+":"+r.id]; ok {
			out = append(out, a)
		}
	}
	return out, total, nil
}

// placeholders returns "?,?,…" for n arguments ("NULL" for none, which
// matches nothing in an IN list).
func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	return "?" + strings.Repeat(",?", n-1)
}

// anys is ids as query arguments.
func anys(ids []string) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

// favoriteSongsFrom selects library songs rated Favourite (the ratings
// sync mirrors every library song's rating as library-songs).
const favoriteSongsFrom = ` FROM tracks t
	JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
	JOIN favorites f ON f.kind = 'library-songs' AND f.apple_id = t.library_id AND f.value = 1`

// FavoriteSongs lists the library songs rated Favourite, most recently
// rated first (then most recently added), and the size of the whole list.
func (l *Library) FavoriteSongs(ctx context.Context, o ListOptions) ([]Song, int, error) {
	limit, offset := o.page()
	songs, err := query(ctx, l, `SELECT `+trackColumns+favoriteSongsFrom+`
		ORDER BY f.updated_at DESC, li.added_rank, t.id LIMIT ? OFFSET ?`, scanSong, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	var total sql.NullInt64
	if err := l.db.R.QueryRowContext(ctx, `SELECT count(*)`+favoriteSongsFrom).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("library: favourite songs: %w", err)
	}
	return songs, int(total.Int64), nil
}
