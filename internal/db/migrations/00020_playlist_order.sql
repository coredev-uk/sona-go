-- The order the user gave their library playlists in the sidebar (internal/
-- library playlists.go, ReorderPlaylist). Apple's web API has no call that
-- sets a playlist's place among its siblings, so the order is Sona's own
-- and stays on this computer; Apple's order (folder_position, which every
-- playlist sync rewrites) is the default and the tiebreak.
--
-- local_position is the playlist's place among the playlists of the folder
-- local_folder ('' is the top level) when the user last ordered them; -1
-- when the user never did. The order counts only while local_folder is the
-- playlist's current folder_id: a playlist that moved to another folder
-- (here or on another device) has no order there until it is placed.

-- +goose Up
ALTER TABLE playlists ADD COLUMN local_folder TEXT NOT NULL DEFAULT '';
ALTER TABLE playlists ADD COLUMN local_position INTEGER NOT NULL DEFAULT -1;

-- +goose Down
ALTER TABLE playlists DROP COLUMN local_position;
ALTER TABLE playlists DROP COLUMN local_folder;
