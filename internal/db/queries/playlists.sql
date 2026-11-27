-- Playlist folders, tree placement, local playlist edits and pins
-- (internal/library playlists.go, internal/services playlists.go).

-- name: UpsertPlaylistFolder :exec
INSERT INTO playlist_folders (library_id, parent_id, name, position, date_added, generation)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (library_id) DO UPDATE SET
    parent_id = excluded.parent_id,
    name = excluded.name,
    position = excluded.position,
    date_added = coalesce(excluded.date_added, playlist_folders.date_added),
    generation = excluded.generation;

-- name: DeleteStalePlaylistFolders :execrows
DELETE FROM playlist_folders WHERE generation < ?;

-- name: ResetPlaylistPlacement :exec
UPDATE playlists SET folder_id = '', folder_position = 1000000, favorited = 0
WHERE library_id IS NOT NULL;

-- name: PlacePlaylist :exec
-- The folder tree (amp-api) carries canDelete and the collaboration flags;
-- the library list (public API) does not.
UPDATE playlists SET folder_id = ?, folder_position = ?, favorited = ?, can_delete = ?,
       is_collaborative = ?, collaborative_host = ?
WHERE library_id = ?;

-- name: SetPlaylistCanDelete :exec
UPDATE playlists SET can_delete = ? WHERE library_id = ?;

-- name: SetPlaylistFolder :exec
UPDATE playlists SET folder_id = ?, folder_position = ? WHERE library_id = ?;

-- name: NextFolderPosition :one
SELECT CAST(coalesce(max(p), -1) + 1 AS INTEGER) FROM (
    SELECT folder_position AS p FROM playlists WHERE folder_id = sqlc.arg(folder_id) AND library_id IS NOT NULL
    UNION ALL
    SELECT position AS p FROM playlist_folders WHERE parent_id = sqlc.arg(folder_id)
);

-- name: UpdatePlaylistDetails :exec
-- An empty artwork_template keeps the stored one.
UPDATE playlists SET name = ?, description = ?, is_public = ?, last_modified = ?, updated_at = ?,
    artwork_template = coalesce(nullif(CAST(sqlc.arg(artwork_template) AS TEXT), ''), artwork_template)
WHERE library_id = sqlc.arg(library_id);

-- name: ListPlaylistFolders :many
SELECT * FROM playlist_folders ORDER BY parent_id, position, name COLLATE NOCASE;

-- name: GetPlaylistFolder :one
SELECT * FROM playlist_folders WHERE library_id = ?;

-- name: RenamePlaylistFolder :exec
UPDATE playlist_folders SET name = ? WHERE library_id = ?;

-- name: DeletePlaylistFolderRow :exec
DELETE FROM playlist_folders WHERE library_id = ?;

-- name: CountFolderChildren :one
SELECT
    (SELECT count(*) FROM playlists p JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
     WHERE p.folder_id = sqlc.arg(folder_id))
  + (SELECT count(*) FROM playlist_folders f WHERE f.parent_id = sqlc.arg(folder_id));

-- name: LibraryPlaylistTree :many
-- local_position is the order the user gave the playlist among its folder's
-- (-1: none, or it was set for another folder).
SELECT p.library_id, p.catalog_id, p.name, p.description, p.curator_name, p.artwork_template,
       p.can_edit, p.can_delete, p.is_public, p.favorited, p.is_collaborative, p.collaborative_host,
       p.track_count, p.last_modified,
       p.folder_id, p.folder_position, li.date_added,
       CAST(CASE WHEN p.local_folder = p.folder_id THEN p.local_position ELSE -1 END AS INTEGER) AS local_position
FROM playlists p
JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
WHERE p.library_id IS NOT NULL
ORDER BY p.folder_id, p.folder_position, p.name COLLATE NOCASE;

-- name: FolderPlaylistOrder :many
-- The library IDs of the playlists of a folder in the order the sidebar lists
-- them: the user's own order first, then the rest in Apple's.
SELECT p.library_id
FROM playlists p
JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
WHERE p.library_id IS NOT NULL AND p.folder_id = sqlc.arg(folder_id)
ORDER BY (p.local_folder <> p.folder_id OR p.local_position < 0), p.local_position,
         p.folder_position, p.name COLLATE NOCASE;

-- name: SetPlaylistLocalOrder :exec
UPDATE playlists SET local_folder = ?, local_position = ? WHERE library_id = ?;

-- name: PlaylistFlags :one
SELECT p.can_delete, p.is_public, p.folder_id
FROM playlists p WHERE p.library_id = ?;

-- name: ClearPins :exec
DELETE FROM pins;

-- name: InsertPin :exec
INSERT INTO pins (kind, apple_id, name, subtitle, artwork_template, position, pinned_at, action)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);

-- name: OrderedPins :many
SELECT p.kind, p.apple_id, p.name, p.subtitle, p.artwork_template, p.action,
       coalesce(t.catalog_id, '') AS song_catalog_id,
       coalesce(t.album_library_id, '') AS song_album_id
FROM pins p
LEFT JOIN tracks t ON p.kind = 'song' AND t.library_id = p.apple_id
ORDER BY p.position;

-- name: AddLibrarySongIfMissing :execrows
INSERT INTO library_items (kind, ref, date_added, added_rank, generation)
VALUES ('song', ?, ?, NULL, ?)
ON CONFLICT (kind, ref) DO NOTHING;

-- name: DropPlaylistItem :exec
DELETE FROM library_items WHERE kind = 'playlist' AND ref = ?;

-- name: GetPlaylistCover :one
SELECT pc.kind, pc.template, pc.upload
FROM playlist_covers pc JOIN playlists p ON p.id = pc.playlist_id
WHERE p.library_id = ?;

-- name: SetPlaylistCover :execrows
INSERT INTO playlist_covers (playlist_id, kind, template, upload, updated_at)
SELECT p.id, sqlc.arg(kind), sqlc.arg(template), sqlc.arg(upload), sqlc.arg(updated_at)
FROM playlists p WHERE p.library_id = sqlc.arg(library_id)
ON CONFLICT (playlist_id) DO UPDATE SET
    kind = excluded.kind,
    template = excluded.template,
    upload = excluded.upload,
    updated_at = excluded.updated_at;

-- name: DeletePlaylistCover :exec
DELETE FROM playlist_covers
WHERE playlist_id IN (SELECT id FROM playlists WHERE library_id = ?);

-- name: ListPlaylistCoverUploads :many
SELECT DISTINCT template AS ref FROM playlist_covers WHERE kind = 'upload'
UNION
SELECT DISTINCT upload AS ref FROM playlist_covers WHERE upload <> '';
