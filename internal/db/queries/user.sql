-- Account, sync checkpoints, ratings mirror, pins and playback history.

-- name: GetAccount :one
SELECT * FROM accounts WHERE id = 1;

-- name: UpsertAccount :exec
INSERT INTO accounts (id, storefront, language, restricted, mut_obtained_at, mut_last_validated_at, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
    storefront = excluded.storefront,
    language = excluded.language,
    restricted = excluded.restricted,
    mut_obtained_at = coalesce(excluded.mut_obtained_at, accounts.mut_obtained_at),
    mut_last_validated_at = coalesce(excluded.mut_last_validated_at, accounts.mut_last_validated_at),
    updated_at = excluded.updated_at;

-- name: SetAccountValidated :exec
UPDATE accounts SET mut_last_validated_at = ?, updated_at = ? WHERE id = 1;

-- name: GetSyncState :one
SELECT * FROM sync_state WHERE domain = ?;

-- name: ListSyncState :many
SELECT * FROM sync_state ORDER BY domain;

-- name: StartSync :exec
INSERT INTO sync_state (domain, generation, in_progress, next_offset, cursor, total, last_started, last_error)
VALUES (?, ?, 1, 0, '', 0, ?, '')
ON CONFLICT (domain) DO UPDATE SET
    generation = excluded.generation,
    in_progress = 1,
    next_offset = 0,
    cursor = '',
    total = 0,
    last_started = excluded.last_started,
    last_error = '';

-- name: ResumeSync :exec
UPDATE sync_state SET last_started = ?, last_error = '' WHERE domain = ?;

-- name: CheckpointSync :exec
UPDATE sync_state SET next_offset = ?, cursor = ?, total = ? WHERE domain = ?;

-- name: FinishSync :exec
-- last_full is set by full runs and kept (NULL) by incremental ones.
UPDATE sync_state
SET in_progress = 0, next_offset = 0, cursor = '', total = ?, last_success = ?,
    last_full = coalesce(?, last_full), last_error = ''
WHERE domain = ?;

-- name: FailSync :exec
UPDATE sync_state SET last_error = ? WHERE domain = ?;

-- name: UpsertFavorite :exec
INSERT INTO favorites (kind, apple_id, value, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT (kind, apple_id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at;

-- name: DeleteFavorite :exec
DELETE FROM favorites WHERE kind = ? AND apple_id = ?;

-- name: DeleteStaleFavorites :execrows
DELETE FROM favorites WHERE kind = ? AND updated_at < ?;

-- name: GetFavorite :one
SELECT value FROM favorites WHERE kind = ? AND apple_id = ?;

-- name: ListFavorites :many
SELECT * FROM favorites WHERE kind = ? ORDER BY updated_at DESC;

-- name: UpsertPin :exec
INSERT INTO pins (kind, apple_id, name, subtitle, artwork_template, position, pinned_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (kind, apple_id) DO UPDATE SET
    name = excluded.name,
    subtitle = excluded.subtitle,
    artwork_template = excluded.artwork_template,
    position = excluded.position;

-- name: DeletePin :exec
DELETE FROM pins WHERE kind = ? AND apple_id = ?;

-- name: ListPins :many
SELECT * FROM pins ORDER BY position, pinned_at;

-- name: InsertHistory :one
INSERT INTO playback_history (
    played_at, item_id, catalog_id, library_id, name, artist_name, album_name,
    artwork_template, duration_ms, played_ms, source
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: SetHistoryPlayed :exec
UPDATE playback_history SET played_ms = ? WHERE id = ?;

-- name: HistorySince :many
-- song_catalog_id is the catalog ID, through the library song for plays of
-- library items.
SELECT h.id, h.played_at, h.catalog_id, h.library_id,
       CAST(coalesce(nullif(h.catalog_id, ''), t.catalog_id, '') AS TEXT) AS song_catalog_id,
       h.name, h.artist_name, h.album_name, h.artwork_template, h.duration_ms, h.played_ms, h.source
FROM playback_history h
LEFT JOIN tracks t ON h.library_id <> '' AND t.library_id = h.library_id
WHERE h.played_at >= ?
ORDER BY h.played_at DESC, h.id DESC
LIMIT ?;

-- name: HistoryLastPlayed :many
SELECT catalog_id, library_id, CAST(max(played_at) AS INTEGER) AS played_at
FROM playback_history
GROUP BY catalog_id, library_id;

-- name: HistoryBefore :one
SELECT EXISTS (SELECT 1 FROM playback_history WHERE played_at < ?);

-- name: TrimHistory :execrows
DELETE FROM playback_history WHERE played_at < ?;
