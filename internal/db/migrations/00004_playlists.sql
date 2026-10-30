-- Playlist folders, playlist placement in the folder tree, and pin actions.

-- +goose Up
-- The user's playlist folders (library-playlist-folders). parent_id is the
-- containing folder's library ID, '' at the root; position orders siblings
-- as Apple lists them. generation is the playlists sync run that last saw
-- the folder; older rows are removed when a run completes.
CREATE TABLE playlist_folders (
    library_id TEXT PRIMARY KEY,
    parent_id  TEXT    NOT NULL DEFAULT '',
    name       TEXT    NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    date_added INTEGER,
    generation INTEGER NOT NULL DEFAULT 0
) WITHOUT ROWID;
CREATE INDEX playlist_folders_parent ON playlist_folders (parent_id, position);

-- Where a library playlist sits in the folder tree ('' = root), what Apple
-- lets the user do with it, and whether it is the Favourite Songs playlist
-- (tag "favorited").
ALTER TABLE playlists ADD COLUMN folder_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playlists ADD COLUMN folder_position INTEGER NOT NULL DEFAULT 0;
ALTER TABLE playlists ADD COLUMN can_delete INTEGER NOT NULL DEFAULT 0;
ALTER TABLE playlists ADD COLUMN favorited INTEGER NOT NULL DEFAULT 0;

-- What activating a sidebar pin does: play, shuffle or drillIn (open).
ALTER TABLE pins ADD COLUMN action TEXT NOT NULL DEFAULT 'play';

-- +goose Down
ALTER TABLE pins DROP COLUMN action;
ALTER TABLE playlists DROP COLUMN favorited;
ALTER TABLE playlists DROP COLUMN can_delete;
ALTER TABLE playlists DROP COLUMN folder_position;
ALTER TABLE playlists DROP COLUMN folder_id;
DROP TABLE playlist_folders;
