-- The artists a library song is credited to. Apple's library song names its
-- first artist only (the library artist "A & B" is one of its own), while the
-- catalogue copy lists each credited artist (include=artists). The sync asks
-- the catalogue, in batches, for the songs whose artist string may name
-- several artists, and keeps the answer here so the Artists list can show a
-- collaboration under each artist. A song with no rows here is listed under
-- its library artist, as before.
--
-- credits_checked is when the catalogue was asked about the song (0: never):
-- an answer with one artist, none, or no answer at all is not asked again; a
-- song whose artist string changes is asked afresh (UpsertLibraryTrack).
--
-- track_credit_keys is the grouping the Artists list reads: each credit under
-- its library artist when the library holds that catalogue artist (by library
-- ID, as the list always keyed), else under the catalogue artist ID; a song
-- without credits under its library artist.

-- +goose Up
ALTER TABLE tracks ADD COLUMN credits_checked INTEGER NOT NULL DEFAULT 0;

CREATE TABLE track_artists (
    track_id          INTEGER NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
    artist_catalog_id TEXT    NOT NULL,
    name              TEXT    NOT NULL,
    position          INTEGER NOT NULL,
    PRIMARY KEY (track_id, artist_catalog_id)
);
CREATE INDEX track_artists_artist ON track_artists (artist_catalog_id);

CREATE VIEW track_credit_keys AS
SELECT ta.track_id AS track_id,
       coalesce((SELECT r.library_id FROM artists r
                 WHERE r.catalog_id = ta.artist_catalog_id AND r.library_id IS NOT NULL
                 ORDER BY r.id LIMIT 1), ta.artist_catalog_id) AS artist_key,
       ta.artist_catalog_id AS credited_catalog_id,
       ta.name AS credited_name
FROM track_artists ta
UNION ALL
SELECT t.id, t.artist_library_id, NULL, NULL
FROM tracks t
WHERE t.artist_library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM track_artists ta WHERE ta.track_id = t.id);

-- +goose Down
DROP VIEW track_credit_keys;
DROP TABLE track_artists;
ALTER TABLE tracks DROP COLUMN credits_checked;
