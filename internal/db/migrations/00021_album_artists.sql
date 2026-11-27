-- The artists a library album is credited to, as 00019 did for songs. Apple's
-- library album names its first artist only (the library artist "A & B" is one
-- of its own), while the catalogue copy lists each credited artist
-- (include=artists). The sync asks the catalogue, in batches, for the albums
-- whose artist string may name several artists, and keeps the answer here so
-- the Artists list can show a collaboration album under each artist. An album
-- with no rows here is listed under its library artist, as before: that is
-- every compilation ("Various Artists": Apple credits it to no artist, so its
-- relationship is empty), every album of one artist, and an album with no
-- catalogue copy.
--
-- credits_checked is when the catalogue was asked about the album (0: never),
-- as on tracks: an answer with one artist, none, or no answer at all is not
-- asked again; an album whose artist string or catalogue ID changes is asked
-- afresh (UpsertLibraryAlbum). No resync is forced: the pass reads what the
-- albums table holds, so the next sync asks once about every library album
-- that may name several.
--
-- album_credit_keys is the grouping the Artists list reads, as
-- track_credit_keys is for songs: each credit under its library artist when
-- the library holds that catalogue artist (by library ID, as the list always
-- keyed), else under the catalogue artist ID; an album without credits under
-- its library artist.

-- +goose Up
ALTER TABLE albums ADD COLUMN credits_checked INTEGER NOT NULL DEFAULT 0;

CREATE TABLE album_artists (
    album_id          INTEGER NOT NULL REFERENCES albums (id) ON DELETE CASCADE,
    artist_catalog_id TEXT    NOT NULL,
    name              TEXT    NOT NULL,
    position          INTEGER NOT NULL,
    PRIMARY KEY (album_id, artist_catalog_id)
);
CREATE INDEX album_artists_artist ON album_artists (artist_catalog_id);

CREATE VIEW album_credit_keys AS
SELECT aa.album_id AS album_id,
       coalesce((SELECT r.library_id FROM artists r
                 WHERE r.catalog_id = aa.artist_catalog_id AND r.library_id IS NOT NULL
                 ORDER BY r.id LIMIT 1), aa.artist_catalog_id) AS artist_key,
       aa.artist_catalog_id AS credited_catalog_id,
       aa.name AS credited_name
FROM album_artists aa
UNION ALL
SELECT a.id, a.artist_library_id, NULL, NULL
FROM albums a
WHERE a.artist_library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM album_artists aa WHERE aa.album_id = a.id);

-- +goose Down
DROP VIEW album_credit_keys;
DROP TABLE album_artists;
ALTER TABLE albums DROP COLUMN credits_checked;
