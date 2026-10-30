-- Discover page cache (internal/services/editorial.go, discover.go).

-- name: GetDiscoverCache :one
SELECT * FROM discover_cache WHERE key = ?;

-- name: PutDiscoverCache :exec
INSERT INTO discover_cache (key, storefront, personal, body, fetched_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET
    storefront = excluded.storefront,
    personal = excluded.personal,
    body = excluded.body,
    fetched_at = excluded.fetched_at,
    expires_at = excluded.expires_at;

-- name: DeleteOldDiscoverCache :execrows
-- Drops entries not refreshed since the cutoff (pages no longer visited).
DELETE FROM discover_cache WHERE fetched_at < ?;
