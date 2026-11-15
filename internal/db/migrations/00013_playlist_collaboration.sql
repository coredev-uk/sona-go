-- +goose Up
-- Whether a library playlist is collaborative (Apple's hasCollaboration) and
-- whether the user hosts it (isCollaborativeHost), from the playlist tree
-- walk. The sidebar lists collaborative playlists under Playlists.
ALTER TABLE playlists ADD COLUMN is_collaborative INTEGER NOT NULL DEFAULT 0;
ALTER TABLE playlists ADD COLUMN collaborative_host INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE playlists DROP COLUMN collaborative_host;
ALTER TABLE playlists DROP COLUMN is_collaborative;
