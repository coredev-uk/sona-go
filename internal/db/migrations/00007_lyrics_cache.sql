-- Lyrics cache (internal/lyrics): each provider's answer for a song, so a
-- song's lyrics are fetched once per provider and changing the provider
-- order or switches needs no new requests. A row with found = 0 records
-- that the provider has no lyrics for the song (kept for a shorter time).
-- Catalogue data like catalog_cache: kept on sign-out, emptied by
-- Settings → Storage → Clear cache.

-- +goose Up
CREATE TABLE lyrics_cache (
    provider   TEXT    NOT NULL, -- config.Lyrics* ID
    track      TEXT    NOT NULL, -- lyrics.Track.Key: "am:<catalog ID>" or a metadata hash
    -- The translation language asked for, for providers whose answer
    -- depends on it; '' otherwise.
    language   TEXT    NOT NULL DEFAULT '',
    found      INTEGER NOT NULL,
    body       BLOB    NOT NULL DEFAULT x'', -- lyrics.Lyrics as JSON when found
    fetched_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (provider, track, language)
) WITHOUT ROWID;
CREATE INDEX lyrics_cache_expires ON lyrics_cache (expires_at);

-- +goose Down
DROP TABLE lyrics_cache;
