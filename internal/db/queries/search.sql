-- Recent searches (newest first).

-- name: ListRecentSearches :many
SELECT * FROM recent_searches ORDER BY searched_at DESC, key LIMIT ?;

-- name: PutRecentSearch :exec
INSERT INTO recent_searches (key, kind, scope, term, item_id, name, subtitle, art_url, searched_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (key) DO UPDATE SET
    term = excluded.term,
    name = excluded.name,
    subtitle = excluded.subtitle,
    art_url = excluded.art_url,
    searched_at = excluded.searched_at;

-- name: TrimRecentSearches :exec
DELETE FROM recent_searches WHERE key NOT IN (
    SELECT key FROM recent_searches ORDER BY searched_at DESC, key LIMIT ?
);

-- name: DeleteRecentSearch :exec
DELETE FROM recent_searches WHERE key = ?;

-- name: ClearRecentSearches :exec
DELETE FROM recent_searches;
