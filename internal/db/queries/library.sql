-- Library-origin rows (library_id set), written by internal/library.

-- name: UpsertLibraryTrack :one
INSERT INTO tracks (
    library_id, catalog_id, name, artist_name, album_name,
    album_library_id, album_catalog_id, artist_library_id, artist_catalog_id,
    artwork_template, duration_ms, track_number, disc_number, release_date,
    genre, content_rating, has_lyrics, isrc, composer, no_play_params, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
    no_play_params = excluded.no_play_params,
    credits_checked = CASE WHEN excluded.artist_name <> tracks.artist_name OR excluded.catalog_id IS NOT tracks.catalog_id THEN 0 ELSE tracks.credits_checked END,
    updated_at = excluded.updated_at
RETURNING id;

-- name: ListTracksToCredit :many
-- The library songs the catalogue was not asked about yet (see 00019): the
-- sync keeps those whose artist string may name several.
SELECT t.id, t.catalog_id, t.artist_name
FROM tracks t JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
WHERE t.catalog_id IS NOT NULL AND t.catalog_id <> '' AND t.credits_checked = 0
ORDER BY t.id;

-- name: DeleteTrackArtists :execrows
DELETE FROM track_artists WHERE track_id = ?;

-- name: InsertTrackArtist :exec
INSERT INTO track_artists (track_id, artist_catalog_id, name, position) VALUES (?, ?, ?, ?)
ON CONFLICT (track_id, artist_catalog_id) DO NOTHING;

-- name: MarkTrackCredited :exec
UPDATE tracks SET credits_checked = ? WHERE id = ?;

-- name: UpsertLibraryAlbum :one
INSERT INTO albums (
    library_id, catalog_id, name, artist_name, artist_library_id, artist_catalog_id,
    artwork_template, release_date, genre, track_count, content_rating, is_single,
    is_prerelease, catalog_release_date, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
    is_single = excluded.is_single,
    is_prerelease = excluded.is_prerelease,
    catalog_release_date = excluded.catalog_release_date,
    credits_checked = CASE WHEN excluded.artist_name <> albums.artist_name OR excluded.catalog_id IS NOT albums.catalog_id THEN 0 ELSE albums.credits_checked END,
    updated_at = excluded.updated_at
RETURNING id;

-- name: ListAlbumsToCredit :many
-- The library albums the catalogue was not asked about yet (see 00021): the
-- sync keeps those whose artist string may name several.
SELECT a.id, a.catalog_id, a.artist_name
FROM albums a JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
WHERE a.catalog_id IS NOT NULL AND a.catalog_id <> '' AND a.credits_checked = 0
ORDER BY a.id;

-- name: DeleteAlbumArtists :execrows
DELETE FROM album_artists WHERE album_id = ?;

-- name: InsertAlbumArtist :exec
INSERT INTO album_artists (album_id, artist_catalog_id, name, position) VALUES (?, ?, ?, ?)
ON CONFLICT (album_id, artist_catalog_id) DO NOTHING;

-- name: MarkAlbumCredited :exec
UPDATE albums SET credits_checked = ? WHERE id = ?;

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

-- name: LibrarySongIDsSince :many
SELECT t.library_id
FROM tracks t
JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
WHERE t.library_id IS NOT NULL AND t.updated_at > ?
ORDER BY t.id;

-- name: LibraryAlbumIDsSince :many
SELECT a.library_id
FROM albums a
JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
WHERE a.library_id IS NOT NULL AND a.updated_at > ?
ORDER BY a.id;

-- name: LibraryPlaylistIDsSince :many
SELECT p.library_id
FROM playlists p
JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
WHERE p.library_id IS NOT NULL AND p.updated_at > ?
ORDER BY p.id;

-- name: LibrarySongKnown :one
SELECT EXISTS (
    SELECT 1 FROM tracks t JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
    WHERE t.library_id = ?
);

-- name: LibraryAlbumKnown :one
SELECT EXISTS (
    SELECT 1 FROM albums a JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
    WHERE a.library_id = ?
);

-- Library albums whose release is still ahead: flagged pre-release by Apple
-- or dated after the day (?1, "YYYY-MM-DD"; a year alone counts only while
-- it is flagged). The incremental sync refreshes them, as it stops at the
-- first album it already has.
-- name: UpcomingLibraryAlbums :many
SELECT a.library_id, a.is_prerelease, a.release_date, a.catalog_release_date
FROM albums a JOIN library_items li ON li.kind = 'album' AND li.ref = a.id
WHERE a.library_id IS NOT NULL
  AND (a.is_prerelease = 1 OR a.release_date > ?1 OR a.catalog_release_date > ?1);

-- The songs Apple gave no playParams yet, whatever their album's state now:
-- a song is asked for until it has them, so one still a placeholder when its
-- album's flag clears (the albums and songs domains run side by side) is not
-- left behind with its stand-in name.
-- name: UnplayableLibrarySongs :many
SELECT t.library_id
FROM tracks t
JOIN library_items li ON li.kind = 'song' AND li.ref = t.id
WHERE t.library_id IS NOT NULL AND t.no_play_params = 1;

-- name: MinSongAddedRank :one
SELECT CAST(coalesce(min(added_rank), 0) AS INTEGER) FROM library_items WHERE kind = 'song';

-- name: CountLibraryKind :one
SELECT count(*) FROM library_items WHERE kind = ?;

-- name: GetTrackByLibraryID :one
SELECT * FROM tracks WHERE library_id = ?;

-- name: GetAlbumByLibraryID :one
SELECT * FROM albums WHERE library_id = ?;

-- name: GetPlaylistByLibraryID :one
SELECT * FROM playlists WHERE library_id = ?;

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
-- The Artists list's entries (Library.ArtistIndex): a library artist unless
-- its songs and albums were all credited elsewhere, and each credited artist
-- the library holds no artist row for (keyed by its catalogue ID).
INSERT INTO search_fts (kind, ref, artwork, name, artist, album)
SELECT 'artist', r.library_id, r.artwork_template, r.name, '', ''
FROM library_items li JOIN artists r ON r.id = li.ref
WHERE li.kind = 'artist'
  AND NOT (
    (r.library_id IN (SELECT t.artist_library_id FROM track_artists ta JOIN tracks t ON t.id = ta.track_id
                      WHERE t.artist_library_id IS NOT NULL)
     OR r.library_id IN (SELECT a.artist_library_id FROM album_artists aa JOIN albums a ON a.id = aa.album_id
                         WHERE a.artist_library_id IS NOT NULL))
    AND r.library_id NOT IN (SELECT k.artist_key FROM album_credit_keys k JOIN library_items x ON x.kind = 'album' AND x.ref = k.album_id
                             WHERE k.artist_key IS NOT NULL)
    AND r.library_id NOT IN (SELECT k.artist_key FROM track_credit_keys k JOIN library_items x ON x.kind = 'song' AND x.ref = k.track_id
                             WHERE k.artist_key IS NOT NULL))
UNION ALL
SELECT 'artist', u.artist_key,
       coalesce((SELECT c.artwork_template FROM artists c WHERE c.catalog_id = u.artist_key AND c.library_id IS NULL LIMIT 1), ''),
       min(u.credited_name), '', ''
FROM (
    SELECT k.artist_key, k.credited_name FROM track_credit_keys k
    JOIN library_items x ON x.kind = 'song' AND x.ref = k.track_id
    WHERE k.credited_catalog_id = k.artist_key
    UNION ALL
    SELECT k.artist_key, k.credited_name FROM album_credit_keys k
    JOIN library_items x ON x.kind = 'album' AND x.ref = k.album_id
    WHERE k.credited_catalog_id = k.artist_key
) u
GROUP BY u.artist_key;

-- name: RebuildSearchPlaylists :exec
INSERT INTO search_fts (kind, ref, artwork, name, artist, album)
SELECT 'playlist', p.library_id, p.artwork_template, p.name, p.curator_name, ''
FROM library_items li JOIN playlists p ON p.id = li.ref
WHERE li.kind = 'playlist';

-- name: UpsertLibraryMusicVideo :exec
INSERT INTO library_music_videos (
    library_id, catalog_id, name, artist_name, album_name, duration_ms, release_date, genre,
    explicit, has_4k, has_hdr, url, artwork_template, artwork_width, artwork_height,
    position, generation, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (library_id) DO UPDATE SET
    catalog_id = excluded.catalog_id,
    name = excluded.name,
    artist_name = excluded.artist_name,
    album_name = excluded.album_name,
    duration_ms = excluded.duration_ms,
    release_date = excluded.release_date,
    genre = excluded.genre,
    explicit = excluded.explicit,
    has_4k = excluded.has_4k,
    has_hdr = excluded.has_hdr,
    url = excluded.url,
    artwork_template = excluded.artwork_template,
    artwork_width = excluded.artwork_width,
    artwork_height = excluded.artwork_height,
    position = excluded.position,
    generation = excluded.generation,
    updated_at = excluded.updated_at;

-- name: ListLibraryMusicVideos :many
SELECT * FROM library_music_videos ORDER BY position, id LIMIT ? OFFSET ?;

-- name: CountLibraryMusicVideos :one
SELECT count(*) FROM library_music_videos;

-- name: DeleteStaleLibraryMusicVideos :execrows
DELETE FROM library_music_videos WHERE generation < ?;

-- name: DeleteLibraryMusicVideo :exec
DELETE FROM library_music_videos WHERE library_id = ?;
