-- Lyrics cache (internal/lyrics).

-- name: GetLyricsCache :one
SELECT * FROM lyrics_cache WHERE provider = ? AND track = ? AND language = ?;

-- name: PutLyricsCache :exec
INSERT INTO lyrics_cache (provider, track, language, found, body, fetched_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (provider, track, language) DO UPDATE SET
    found = excluded.found,
    body = excluded.body,
    fetched_at = excluded.fetched_at,
    expires_at = excluded.expires_at;

-- name: DeleteExpiredLyricsCache :execrows
DELETE FROM lyrics_cache WHERE expires_at < ?;

-- name: LyricsCacheSize :one
SELECT count(*) AS entries, CAST(coalesce(sum(length(body)), 0) AS INTEGER) AS bytes FROM lyrics_cache;

-- name: ClearLyricsCache :execrows
DELETE FROM lyrics_cache;
