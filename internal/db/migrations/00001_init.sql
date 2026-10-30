-- Sona's local store. Plain SQLite (libSQL-compatible): no extensions beyond
-- FTS5. Timestamps are Unix milliseconds. Apple catalog and library IDs live
-- in separate columns; a row with library_id NULL is catalog metadata cached
-- by internal/catalog, a row with library_id set came from the user's library.

-- +goose Up
CREATE TABLE accounts (
    id                    INTEGER PRIMARY KEY CHECK (id = 1),
    storefront            TEXT    NOT NULL,
    language              TEXT    NOT NULL DEFAULT '',
    restricted            INTEGER NOT NULL DEFAULT 0,
    mut_obtained_at       INTEGER,
    mut_last_validated_at INTEGER,
    updated_at            INTEGER NOT NULL
);

CREATE TABLE artists (
    id               INTEGER PRIMARY KEY,
    library_id       TEXT UNIQUE,
    catalog_id       TEXT,
    name             TEXT    NOT NULL,
    artwork_template TEXT    NOT NULL DEFAULT '',
    url              TEXT    NOT NULL DEFAULT '',
    updated_at       INTEGER NOT NULL
);
CREATE UNIQUE INDEX artists_catalog_only ON artists (catalog_id) WHERE library_id IS NULL;
CREATE INDEX artists_catalog_id ON artists (catalog_id);
CREATE INDEX artists_name ON artists (name COLLATE NOCASE);

CREATE TABLE albums (
    id                INTEGER PRIMARY KEY,
    library_id        TEXT UNIQUE,
    catalog_id        TEXT,
    name              TEXT    NOT NULL,
    artist_name       TEXT    NOT NULL DEFAULT '',
    artist_library_id TEXT,
    artist_catalog_id TEXT,
    artwork_template  TEXT    NOT NULL DEFAULT '',
    release_date      TEXT    NOT NULL DEFAULT '',
    genre             TEXT    NOT NULL DEFAULT '',
    track_count       INTEGER NOT NULL DEFAULT 0,
    content_rating    TEXT    NOT NULL DEFAULT '',
    is_single         INTEGER NOT NULL DEFAULT 0,
    is_compilation    INTEGER NOT NULL DEFAULT 0,
    record_label      TEXT    NOT NULL DEFAULT '',
    copyright         TEXT    NOT NULL DEFAULT '',
    editorial_notes   TEXT    NOT NULL DEFAULT '',
    url               TEXT    NOT NULL DEFAULT '',
    updated_at        INTEGER NOT NULL
);
CREATE UNIQUE INDEX albums_catalog_only ON albums (catalog_id) WHERE library_id IS NULL;
CREATE INDEX albums_catalog_id ON albums (catalog_id);
CREATE INDEX albums_name ON albums (name COLLATE NOCASE);
CREATE INDEX albums_artist ON albums (artist_name COLLATE NOCASE, name COLLATE NOCASE);
CREATE INDEX albums_artist_library_id ON albums (artist_library_id);

CREATE TABLE tracks (
    id                INTEGER PRIMARY KEY,
    library_id        TEXT UNIQUE,
    catalog_id        TEXT,
    name              TEXT    NOT NULL,
    artist_name       TEXT    NOT NULL DEFAULT '',
    album_name        TEXT    NOT NULL DEFAULT '',
    album_library_id  TEXT,
    album_catalog_id  TEXT,
    artist_library_id TEXT,
    artist_catalog_id TEXT,
    artwork_template  TEXT    NOT NULL DEFAULT '',
    duration_ms       INTEGER NOT NULL DEFAULT 0,
    track_number      INTEGER NOT NULL DEFAULT 0,
    disc_number       INTEGER NOT NULL DEFAULT 0,
    release_date      TEXT    NOT NULL DEFAULT '',
    genre             TEXT    NOT NULL DEFAULT '',
    content_rating    TEXT    NOT NULL DEFAULT '',
    has_lyrics        INTEGER NOT NULL DEFAULT 0,
    isrc              TEXT    NOT NULL DEFAULT '',
    composer          TEXT    NOT NULL DEFAULT '',
    updated_at        INTEGER NOT NULL
);
CREATE UNIQUE INDEX tracks_catalog_only ON tracks (catalog_id) WHERE library_id IS NULL;
CREATE INDEX tracks_catalog_id ON tracks (catalog_id);
CREATE INDEX tracks_name ON tracks (name COLLATE NOCASE);
CREATE INDEX tracks_artist ON tracks (artist_name COLLATE NOCASE, album_name COLLATE NOCASE, disc_number, track_number);
CREATE INDEX tracks_album ON tracks (album_name COLLATE NOCASE, disc_number, track_number);
CREATE INDEX tracks_album_library_id ON tracks (album_library_id, disc_number, track_number);
CREATE INDEX tracks_album_catalog_id ON tracks (album_catalog_id, disc_number, track_number);

CREATE TABLE playlists (
    id               INTEGER PRIMARY KEY,
    library_id       TEXT UNIQUE,
    catalog_id       TEXT,
    name             TEXT    NOT NULL,
    description      TEXT    NOT NULL DEFAULT '',
    curator_name     TEXT    NOT NULL DEFAULT '',
    artwork_template TEXT    NOT NULL DEFAULT '',
    can_edit         INTEGER NOT NULL DEFAULT 0,
    is_public        INTEGER NOT NULL DEFAULT 0,
    last_modified    TEXT    NOT NULL DEFAULT '',
    -- last_modified as of the last successful track sync; equal means the
    -- stored playlist_tracks are current.
    tracks_modified  TEXT    NOT NULL DEFAULT '',
    track_count      INTEGER NOT NULL DEFAULT 0,
    url              TEXT    NOT NULL DEFAULT '',
    updated_at       INTEGER NOT NULL
);
CREATE UNIQUE INDEX playlists_catalog_only ON playlists (catalog_id) WHERE library_id IS NULL;
CREATE INDEX playlists_name ON playlists (name COLLATE NOCASE);

CREATE TABLE playlist_tracks (
    playlist_id INTEGER NOT NULL REFERENCES playlists (id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    track_id    INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
    PRIMARY KEY (playlist_id, position)
) WITHOUT ROWID;
CREATE INDEX playlist_tracks_track ON playlist_tracks (track_id);

-- Membership of the user's library. ref is the rowid in the table named by
-- kind (song → tracks, album → albums, artist → artists, playlist →
-- playlists). generation is the sync run that last saw the item; rows older
-- than a completed run are removed.
CREATE TABLE library_items (
    kind       TEXT    NOT NULL CHECK (kind IN ('song', 'album', 'artist', 'playlist')),
    ref        INTEGER NOT NULL,
    date_added INTEGER,
    added_rank INTEGER,
    generation INTEGER NOT NULL,
    PRIMARY KEY (kind, ref)
) WITHOUT ROWID;
CREATE INDEX library_items_rank ON library_items (kind, added_rank);
CREATE INDEX library_items_added ON library_items (kind, date_added);
CREATE INDEX library_items_generation ON library_items (kind, generation);

CREATE TABLE playback_history (
    id               INTEGER PRIMARY KEY,
    played_at        INTEGER NOT NULL,
    item_id          TEXT    NOT NULL,
    catalog_id       TEXT    NOT NULL DEFAULT '',
    library_id       TEXT    NOT NULL DEFAULT '',
    name             TEXT    NOT NULL DEFAULT '',
    artist_name      TEXT    NOT NULL DEFAULT '',
    album_name       TEXT    NOT NULL DEFAULT '',
    artwork_template TEXT    NOT NULL DEFAULT '',
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    played_ms        INTEGER NOT NULL DEFAULT 0,
    source           TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX playback_history_played_at ON playback_history (played_at);

-- Mirrors /v1/me/ratings. kind is Apple's ratings type (songs,
-- library-songs, albums, library-albums, playlists, library-playlists).
CREATE TABLE favorites (
    kind       TEXT    NOT NULL,
    apple_id   TEXT    NOT NULL,
    value      INTEGER NOT NULL CHECK (value IN (-1, 1)),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (kind, apple_id)
) WITHOUT ROWID;

CREATE TABLE pins (
    kind             TEXT    NOT NULL,
    apple_id         TEXT    NOT NULL,
    name             TEXT    NOT NULL DEFAULT '',
    subtitle         TEXT    NOT NULL DEFAULT '',
    artwork_template TEXT    NOT NULL DEFAULT '',
    position         INTEGER NOT NULL DEFAULT 0,
    pinned_at        INTEGER NOT NULL,
    PRIMARY KEY (kind, apple_id)
) WITHOUT ROWID;

-- One row per library sync domain. A run in progress keeps its generation
-- and next_offset (all items before it are stored), so an interrupted run
-- resumes where it stopped.
CREATE TABLE sync_state (
    domain       TEXT PRIMARY KEY,
    generation   INTEGER NOT NULL DEFAULT 0,
    in_progress  INTEGER NOT NULL DEFAULT 0,
    next_offset  INTEGER NOT NULL DEFAULT 0,
    cursor       TEXT    NOT NULL DEFAULT '',
    total        INTEGER NOT NULL DEFAULT 0,
    last_started INTEGER,
    last_success INTEGER,
    last_error   TEXT    NOT NULL DEFAULT ''
);

-- Local search over the library (ref is the Apple library ID). Rebuilt
-- from library_items after a sync.
CREATE VIRTUAL TABLE search_fts USING fts5 (
    kind UNINDEXED,
    ref UNINDEXED,
    artwork UNINDEXED,
    name,
    artist,
    album,
    tokenize = 'unicode61 remove_diacritics 2',
    prefix = '2 3'
);

-- Raw catalog responses. key includes the storefront and request; catalog
-- data is not user-specific and survives sign-out.
CREATE TABLE catalog_cache (
    key        TEXT PRIMARY KEY,
    storefront TEXT    NOT NULL,
    body       BLOB    NOT NULL,
    etag       TEXT    NOT NULL DEFAULT '',
    fetched_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX catalog_cache_expires ON catalog_cache (expires_at);

-- +goose Down
DROP TABLE catalog_cache;
DROP TABLE search_fts;
DROP TABLE sync_state;
DROP TABLE pins;
DROP TABLE favorites;
DROP TABLE playback_history;
DROP TABLE library_items;
DROP TABLE playlist_tracks;
DROP TABLE playlists;
DROP TABLE tracks;
DROP TABLE albums;
DROP TABLE artists;
DROP TABLE accounts;
