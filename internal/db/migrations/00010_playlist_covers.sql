-- The cover the user chose for a library playlist (internal/library
-- covers.go). Apple's web API has no way to set a playlist's artwork, so the
-- choice is Sona's own and stays on this computer.
--
-- A playlist without a row has the automatic cover: Apple's artwork, or a
-- mosaic of its tracks' albums. kind says what is shown instead: 'none' (the
-- plain placeholder), 'preset' (template is a generated cover) or 'upload'
-- (template names the image the user chose, kept in the covers directory).
-- upload is the last image the user chose, kept while another cover is
-- shown so that going back needs no new file; '' when there is none. A
-- row with kind 'auto' exists only to hold such an upload.

-- +goose Up
CREATE TABLE playlist_covers (
    playlist_id INTEGER PRIMARY KEY REFERENCES playlists (id) ON DELETE CASCADE,
    kind        TEXT    NOT NULL CHECK (kind IN ('auto', 'none', 'preset', 'upload')),
    template    TEXT    NOT NULL DEFAULT '',
    upload      TEXT    NOT NULL DEFAULT '',
    updated_at  INTEGER NOT NULL
);

-- +goose Down
DROP TABLE playlist_covers;
