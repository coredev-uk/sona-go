-- Catalog-origin rows (library_id NULL) and the raw response cache, written
-- by internal/catalog.

-- name: UpsertCatalogTrack :one
INSERT INTO tracks (
    catalog_id, name, artist_name, album_name, album_catalog_id, artist_catalog_id,
    artwork_template, duration_ms, track_number, disc_number, release_date,
    genre, content_rating, has_lyrics, isrc, composer, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (catalog_id) WHERE library_id IS NULL DO UPDATE SET
    name = excluded.name,
    artist_name = excluded.artist_name,
    album_name = excluded.album_name,
    album_catalog_id = coalesce(excluded.album_catalog_id, tracks.album_catalog_id),
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

-- name: UpsertCatalogAlbum :one
INSERT INTO albums (
    catalog_id, name, artist_name, artist_catalog_id, artwork_template, release_date,
    genre, track_count, content_rating, is_single, is_compilation, record_label,
    copyright, editorial_notes, url, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (catalog_id) WHERE library_id IS NULL DO UPDATE SET
    name = excluded.name,
    artist_name = excluded.artist_name,
    artist_catalog_id = coalesce(excluded.artist_catalog_id, albums.artist_catalog_id),
    artwork_template = excluded.artwork_template,
    release_date = excluded.release_date,
    genre = excluded.genre,
    track_count = excluded.track_count,
    content_rating = excluded.content_rating,
    is_single = excluded.is_single,
    is_compilation = excluded.is_compilation,
    record_label = excluded.record_label,
    copyright = excluded.copyright,
    editorial_notes = excluded.editorial_notes,
    url = excluded.url,
    updated_at = excluded.updated_at
RETURNING id;

-- name: UpsertCatalogArtist :one
INSERT INTO artists (catalog_id, name, artwork_template, url, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (catalog_id) WHERE library_id IS NULL DO UPDATE SET
    name = excluded.name,
    artwork_template = excluded.artwork_template,
    url = excluded.url,
    updated_at = excluded.updated_at
RETURNING id;

-- name: UpsertCatalogPlaylist :one
INSERT INTO playlists (catalog_id, name, description, curator_name, artwork_template, last_modified, track_count, url, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (catalog_id) WHERE library_id IS NULL DO UPDATE SET
    name = excluded.name,
    description = excluded.description,
    curator_name = excluded.curator_name,
    artwork_template = excluded.artwork_template,
    last_modified = excluded.last_modified,
    track_count = excluded.track_count,
    url = excluded.url,
    updated_at = excluded.updated_at
RETURNING id;

-- name: GetTrackByCatalogID :one
-- Prefers the library row when the song is in the library.
SELECT * FROM tracks WHERE catalog_id = ? ORDER BY library_id IS NULL, id LIMIT 1;

-- name: GetCatalogAlbum :one
SELECT * FROM albums WHERE catalog_id = ? AND library_id IS NULL;

-- name: GetCache :one
SELECT * FROM catalog_cache WHERE key = ?;

-- name: PutCache :exec
INSERT INTO catalog_cache (key, storefront, body, etag, fetched_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET
    storefront = excluded.storefront,
    body = excluded.body,
    etag = excluded.etag,
    fetched_at = excluded.fetched_at,
    expires_at = excluded.expires_at;

-- name: DeleteCache :exec
DELETE FROM catalog_cache WHERE key = ?;

-- name: DeleteExpiredCache :execrows
DELETE FROM catalog_cache WHERE expires_at < ?;

-- name: CatalogCacheSize :one
SELECT count(*) AS entries, CAST(coalesce(sum(length(body)), 0) AS INTEGER) AS bytes FROM catalog_cache;

-- name: ClearCatalogCache :execrows
DELETE FROM catalog_cache;
