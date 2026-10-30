-- Library-origin rows (library_id set), written by internal/library.

-- name: UpsertLibraryTrack :one
INSERT INTO tracks (
    library_id, catalog_id, name, artist_name, album_name,
    album_library_id, album_catalog_id, artist_library_id, artist_catalog_id,
    artwork_template, duration_ms, track_number, disc_number, release_date,
    genre, content_rating, has_lyrics, isrc, composer, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (library_id) DO UPDATE SET
    catalog_id = excluded.catalog_id,
    name = excluded.name,
    artist_name = excluded.artist_name,
    album_name = excluded.album_name,
    album_library_id = coalesce(excluded.album_library_id, tracks.album_library_id),
    album_catalog_id = coalesce(excluded.album_catalog_id, tracks.album_catalog_id),
    artist_library_id = coalesce(excluded.artist_library_id, tracks.artist_library_id),
    artist_catalog_id = coalesce(excluded.artist_catalog_id, tracks.artist_catalog_id),
    artwork_template = excluded.artwork_template,
    duration_ms = excluded.duration_ms,
    track_number = excluded.track_number,
    disc_number = excluded.disc_number,
    release_date = excluded.release_date,
    genre = excluded.genre,
    content_rating = excluded.content_rating,
    has_lyrics = excluded.has_lyrics,
    isrc = excluded.isrc,
    composer = excluded.composer,
    updated_at = excluded.updated_at
RETURNING id;

-- name: UpsertLibraryAlbum :one
INSERT INTO albums (
    library_id, catalog_id, name, artist_name, artist_library_id, artist_catalog_id,
    artwork_template, release_date, genre, track_count, content_rating, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (library_id) DO UPDATE SET
    catalog_id = excluded.catalog_id,
    name = excluded.name,
    artist_name = excluded.artist_name,
    artist_library_id = coalesce(excluded.artist_library_id, albums.artist_library_id),
    artist_catalog_id = coalesce(excluded.artist_catalog_id, albums.artist_catalog_id),
    artwork_template = excluded.artwork_template,
    release_date = excluded.release_date,
    genre = excluded.genre,
    track_count = excluded.track_count,
    content_rating = excluded.content_rating,
    updated_at = excluded.updated_at
RETURNING id;

-- name: UpsertLibraryArtist :one
INSERT INTO artists (library_id, catalog_id, name, artwork_template, url, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (library_id) DO UPDATE SET
    catalog_id = excluded.catalog_id,
    name = excluded.name,
    artwork_template = excluded.artwork_template,
    url = excluded.url,
    updated_at = excluded.updated_at
RETURNING id;

-- name: UpsertLibraryPlaylist :one
INSERT INTO playlists (
    library_id, catalog_id, name, description, curator_name, artwork_template,
    can_edit, is_public, last_modified, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (library_id) DO UPDATE SET
    catalog_id = excluded.catalog_id,
    name = excluded.name,
    description = excluded.description,
    curator_name = excluded.curator_name,
    artwork_template = excluded.artwork_template,
    can_edit = excluded.can_edit,
    is_public = excluded.is_public,
    last_modified = excluded.last_modified,
    updated_at = excluded.updated_at
RETURNING id;

-- name: MarkLibraryItem :exec
INSERT INTO library_items (kind, ref, date_added, added_rank, generation)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (kind, ref) DO UPDATE SET
    date_added = coalesce(excluded.date_added, library_items.date_added),
    added_rank = excluded.added_rank,
    generation = excluded.generation;

-- name: DeleteStaleLibraryItems :execrows
DELETE FROM library_items WHERE kind = ? AND generation < ?;

-- name: DeleteOrphanLibraryTracks :execrows
DELETE FROM tracks
WHERE library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM library_items li WHERE li.kind = 'song' AND li.ref = tracks.id)
  AND NOT EXISTS (SELECT 1 FROM playlist_tracks pt WHERE pt.track_id = tracks.id);

-- name: DeleteOrphanLibraryAlbums :execrows
DELETE FROM albums
WHERE library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM library_items li WHERE li.kind = 'album' AND li.ref = albums.id);

-- name: DeleteOrphanLibraryArtists :execrows
DELETE FROM artists
WHERE library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM library_items li WHERE li.kind = 'artist' AND li.ref = artists.id);

-- name: DeleteOrphanLibraryPlaylists :execrows
DELETE FROM playlists
WHERE library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM library_items li WHERE li.kind = 'playlist' AND li.ref = playlists.id);

-- name: LibraryPlaylistsForTrackSync :many
SELECT p.id, p.library_id, p.last_modified, p.tracks_modified
FROM playlists p
JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
ORDER BY p.id;

-- name: DeletePlaylistTracks :exec
DELETE FROM playlist_tracks WHERE playlist_id = ?;

-- name: InsertPlaylistTrack :exec
INSERT INTO playlist_tracks (playlist_id, position, track_id) VALUES (?, ?, ?);

-- name: SetPlaylistTracksSynced :exec
UPDATE playlists SET tracks_modified = ?, track_count = ? WHERE id = ?;

-- name: LibrarySongIDs :many
SELECT t.library_id
FROM tracks t
JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
WHERE t.library_id IS NOT NULL
ORDER BY t.id;

-- name: LibraryAlbumIDs :many
SELECT a.library_id
FROM albums a
JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
WHERE a.library_id IS NOT NULL
ORDER BY a.id;

-- name: LibraryPlaylistIDs :many
SELECT p.library_id
FROM playlists p
JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
WHERE p.library_id IS NOT NULL
ORDER BY p.id;

-- name: CountLibraryItems :many
SELECT kind, count(*) AS n FROM library_items GROUP BY kind;

-- name: GetTrackByLibraryID :one
SELECT * FROM tracks WHERE library_id = ?;

-- name: GetAlbumByLibraryID :one
SELECT * FROM albums WHERE library_id = ?;

-- name: GetArtistByLibraryID :one
SELECT * FROM artists WHERE library_id = ?;

-- name: GetPlaylistByLibraryID :one
SELECT * FROM playlists WHERE library_id = ?;

-- name: LibraryAlbumTracks :many
SELECT * FROM tracks
WHERE album_library_id = ?
ORDER BY disc_number, track_number, name COLLATE NOCASE;

-- name: LibraryArtistTracks :many
SELECT t.* FROM tracks t
JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
WHERE t.artist_library_id = ?
ORDER BY t.album_name COLLATE NOCASE, t.disc_number, t.track_number;

-- name: ClearSearchIndex :exec
DELETE FROM search_fts;

-- name: RebuildSearchTracks :exec
INSERT INTO search_fts (kind, ref, artwork, name, artist, album)
SELECT 'song', t.library_id, t.artwork_template, t.name, t.artist_name, t.album_name
FROM library_items li JOIN tracks t ON t.id = li.ref
WHERE li.kind = 'song';

-- name: RebuildSearchAlbums :exec
INSERT INTO search_fts (kind, ref, artwork, name, artist, album)
SELECT 'album', a.library_id, a.artwork_template, a.name, a.artist_name, ''
FROM library_items li JOIN albums a ON a.id = li.ref
WHERE li.kind = 'album';

-- name: RebuildSearchArtists :exec
INSERT INTO search_fts (kind, ref, artwork, name, artist, album)
SELECT 'artist', r.library_id, r.artwork_template, r.name, '', ''
FROM library_items li JOIN artists r ON r.id = li.ref
WHERE li.kind = 'artist';

-- name: RebuildSearchPlaylists :exec
INSERT INTO search_fts (kind, ref, artwork, name, artist, album)
SELECT 'playlist', p.library_id, p.artwork_template, p.name, p.curator_name, ''
FROM library_items li JOIN playlists p ON p.id = li.ref
WHERE li.kind = 'playlist';
