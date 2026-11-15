-- The artist tracker (services/artisttracker.go).

-- name: ListLibraryArtistCatalogIDs :many
SELECT DISTINCT r.catalog_id
FROM library_items li JOIN artists r ON r.id = li.ref
WHERE li.kind = 'artist' AND r.catalog_id IS NOT NULL AND r.catalog_id <> ''
ORDER BY r.catalog_id;

-- name: ListFavouriteArtistIDs :many
SELECT apple_id FROM favorites WHERE kind = 'artists' AND value = 1 ORDER BY apple_id;

-- name: ListTrackedArtists :many
SELECT artist_id FROM tracked_artists ORDER BY artist_id;

-- name: PutTrackedArtist :exec
INSERT INTO tracked_artists (artist_id, name, seeded_at) VALUES (?, ?, ?)
ON CONFLICT (artist_id) DO UPDATE SET name = excluded.name;

-- name: DeleteTrackedArtist :exec
DELETE FROM tracked_artists WHERE artist_id = ?;

-- name: DeleteArtistReleasesOf :exec
DELETE FROM artist_releases WHERE artist_id = ?;

-- name: ClearTrackedArtists :exec
DELETE FROM tracked_artists;

-- name: ListSeenReleaseIDs :many
SELECT album_id FROM artist_releases;

-- name: PutArtistRelease :exec
-- A release seen for the first time; when it is already known, only what
-- Apple may change (its date, its pre-release flag, its name and cover)
-- moves, never the tracker's or the user's own columns.
INSERT INTO artist_releases (album_id, artist_id, name, artist_name, artwork_template, kind, release_date, release_at,
                             prerelease, track_count, explicit, listed, found_at, read_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (album_id) DO UPDATE SET
    name = excluded.name,
    artist_name = excluded.artist_name,
    artwork_template = excluded.artwork_template,
    kind = excluded.kind,
    release_date = excluded.release_date,
    release_at = excluded.release_at,
    prerelease = excluded.prerelease,
    track_count = excluded.track_count,
    explicit = excluded.explicit;

-- name: ListArtistReleases :many
SELECT * FROM artist_releases
WHERE listed = 1 AND dismissed_at IS NULL
ORDER BY CASE WHEN release_at > 0 THEN release_at ELSE found_at END DESC, found_at DESC, album_id;

-- name: MarkArtistReleaseRead :exec
UPDATE artist_releases SET read_at = ? WHERE album_id = ? AND read_at IS NULL;

-- name: MarkArtistReleasesRead :exec
UPDATE artist_releases SET read_at = ? WHERE listed = 1 AND dismissed_at IS NULL AND read_at IS NULL;

-- name: DismissArtistRelease :exec
UPDATE artist_releases SET dismissed_at = ? WHERE album_id = ? AND dismissed_at IS NULL;

-- name: DismissArtistReleases :exec
UPDATE artist_releases SET dismissed_at = ? WHERE listed = 1 AND dismissed_at IS NULL;

-- name: PruneArtistReleases :exec
DELETE FROM artist_releases WHERE found_at < ?;

-- name: GetArtistTrackerChecked :one
SELECT checked_at FROM artist_tracker_state WHERE id = 1;

-- name: PutArtistTrackerChecked :exec
INSERT INTO artist_tracker_state (id, checked_at) VALUES (1, ?)
ON CONFLICT (id) DO UPDATE SET checked_at = excluded.checked_at;

-- name: ClearArtistReleases :exec
DELETE FROM artist_releases;

-- name: ClearArtistTrackerState :exec
DELETE FROM artist_tracker_state;
