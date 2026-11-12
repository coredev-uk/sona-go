package library

import (
	"context"
)

// Results are library search matches resolved to full rows, best match
// first within each kind. A kind's More is set when it has further matches
// beyond the requested number.
type Results struct {
	Songs         []Song
	Albums        []Album
	Artists       []Artist
	Playlists     []Playlist
	SongsMore     bool
	AlbumsMore    bool
	ArtistsMore   bool
	PlaylistsMore bool
}

// SearchAll searches the library like Search and returns up to perKind
// matches of every kind as full rows (songs, albums, artists and
// playlists, each in its best-match order).
func (l *Library) SearchAll(ctx context.Context, term string, perKind int) (Results, error) {
	r := Results{Songs: []Song{}, Albums: []Album{}, Artists: []Artist{}, Playlists: []Playlist{}}
	if perKind <= 0 {
		perKind = 25
	}
	ids := func(kind string) ([]string, bool, error) {
		hits, err := l.Search(ctx, term, perKind+1, kind)
		if err != nil {
			return nil, false, err
		}
		more := len(hits) > perKind
		hits = hits[:min(len(hits), perKind)]
		out := make([]string, len(hits))
		for i, h := range hits {
			out[i] = h.LibraryID
		}
		return out, more, nil
	}
	var err error
	var songIDs, albumIDs, artistIDs, playlistIDs []string
	if songIDs, r.SongsMore, err = ids("song"); err != nil {
		return r, err
	}
	if albumIDs, r.AlbumsMore, err = ids("album"); err != nil {
		return r, err
	}
	if artistIDs, r.ArtistsMore, err = ids("artist"); err != nil {
		return r, err
	}
	if playlistIDs, r.PlaylistsMore, err = ids("playlist"); err != nil {
		return r, err
	}
	if r.Songs, err = byIDs(ctx, l, `SELECT `+trackColumns+` FROM tracks t WHERE t.library_id IN `, songIDs, scanSong,
		func(s Song) string { return s.LibraryID }); err != nil {
		return r, err
	}
	if r.Albums, err = byIDs(ctx, l, albumsByID, albumIDs, scanAlbum, func(a Album) string { return a.LibraryID }); err != nil {
		return r, err
	}
	if r.Artists, err = byIDs(ctx, l, `SELECT r.library_id, r.catalog_id, r.name, r.artwork_template FROM artists r
		WHERE r.library_id IN `, artistIDs, scanArtist, func(a Artist) string { return a.LibraryID }); err != nil {
		return r, err
	}
	if r.Playlists, err = byIDs(ctx, l, playlistsByID, playlistIDs, scanPlaylist, func(p Playlist) string { return p.LibraryID }); err != nil {
		return r, err
	}
	if err := l.FillCovers(ctx, r.Playlists); err != nil {
		return r, err
	}
	return r, nil
}

// The library albums and playlists of given library IDs (queries ending in
// "IN ", for byIDs).
const (
	albumsByID = `SELECT ` + albumColumns + ` FROM albums a
		JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
		WHERE a.library_id IN `
	playlistsByID = `SELECT ` + playlistColumns + ` FROM playlists p
		JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		WHERE p.library_id IN `
)

// byIDs runs q (ending in "IN ") for ids and returns the rows in the order
// of ids, skipping IDs without a row.
func byIDs[T any](ctx context.Context, l *Library, q string, ids []string,
	scan func(scanner) (T, error), id func(T) string,
) ([]T, error) {
	if len(ids) == 0 {
		return []T{}, nil
	}
	rows, err := query(ctx, l, q+`(`+placeholders(len(ids))+`)`, scan, anys(ids)...)
	if err != nil {
		return nil, err
	}
	at := make(map[string]T, len(rows))
	for _, r := range rows {
		at[id(r)] = r
	}
	out := make([]T, 0, len(ids))
	for _, v := range ids {
		if r, ok := at[v]; ok {
			out = append(out, r)
		}
	}
	return out, nil
}
