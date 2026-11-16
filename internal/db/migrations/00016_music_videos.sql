-- The library's music videos (library.DomainMusicVideos), copied from Apple
-- so Library → Music videos lists them with no connection. User data, cleared
-- by WipeUserData on sign-out.
--
-- library_id is Apple's library ID ("i.…"); catalog_id the catalogue video's
-- (empty when Apple gave none). position is the video's place in Apple's list
-- (the order the page shows), and generation the run that last saw it, so a
-- full run removes what Apple no longer lists. artwork_template, artwork_width
-- and artwork_height are Apple's 16:9 frame as given (a template with {w}x{h}).

-- +goose Up
CREATE TABLE library_music_videos (
    id               INTEGER PRIMARY KEY,
    library_id       TEXT    NOT NULL UNIQUE,
    catalog_id       TEXT    NOT NULL DEFAULT '',
    name             TEXT    NOT NULL DEFAULT '',
    artist_name      TEXT    NOT NULL DEFAULT '',
    album_name       TEXT    NOT NULL DEFAULT '',
    duration_ms      INTEGER NOT NULL DEFAULT 0,
    release_date     TEXT    NOT NULL DEFAULT '',
    genre            TEXT    NOT NULL DEFAULT '',
    explicit         INTEGER NOT NULL DEFAULT 0,
    has_4k           INTEGER NOT NULL DEFAULT 0,
    has_hdr          INTEGER NOT NULL DEFAULT 0,
    url              TEXT    NOT NULL DEFAULT '',
    artwork_template TEXT    NOT NULL DEFAULT '',
    artwork_width    INTEGER NOT NULL DEFAULT 0,
    artwork_height   INTEGER NOT NULL DEFAULT 0,
    position         INTEGER NOT NULL DEFAULT 0,
    generation       INTEGER NOT NULL DEFAULT 0,
    updated_at       INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX library_music_videos_position ON library_music_videos (position);

-- +goose Down
DROP TABLE library_music_videos;
