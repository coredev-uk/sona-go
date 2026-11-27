-- A credit goes to the library artist of that name, not to a combined one.
--
-- Apple's library artist for "A & B" carries A's catalogue ID, so the views of
-- 00019 and 00021 (which resolved a credited catalogue artist to "the library
-- artist with that catalogue ID, the oldest first") gave A's credits to the
-- combined "A & B" row whenever A had no library artist of their own, and to
-- whichever row was inserted first when A had one. A credit now resolves to a
-- library artist only when that artist is the credited one: the same
-- catalogue ID and the same name, compared case-, accent- and width-folded as
-- the song page does (internal/namefold). Any other credit is keyed by its
-- catalogue ID, as a credit-only artist, and a combined row keeps only what is
-- uncredited (an upload, a song with no catalogue copy).
--
-- SQLite cannot fold Unicode, so the folded name is stored: name_key, on the
-- artists and on each credit, written with the row (the sync) and, for the
-- rows already stored, by migration 00023, which also asks the catalogue about
-- the credits again for the artists that no row holds a picture for (the
-- credits pass now keeps the artists' name and artwork from the answer it
-- already gets).

-- +goose Up
ALTER TABLE artists ADD COLUMN name_key TEXT NOT NULL DEFAULT '';
ALTER TABLE track_artists ADD COLUMN name_key TEXT NOT NULL DEFAULT '';
ALTER TABLE album_artists ADD COLUMN name_key TEXT NOT NULL DEFAULT '';
CREATE INDEX artists_credit_key ON artists (catalog_id, name_key) WHERE library_id IS NOT NULL;

DROP VIEW track_credit_keys;
CREATE VIEW track_credit_keys AS
SELECT ta.track_id AS track_id,
       coalesce((SELECT r.library_id FROM artists r
                 WHERE r.library_id IS NOT NULL AND r.catalog_id = ta.artist_catalog_id AND r.name_key = ta.name_key
                 ORDER BY r.id LIMIT 1), ta.artist_catalog_id) AS artist_key,
       ta.artist_catalog_id AS credited_catalog_id,
       ta.name AS credited_name
FROM track_artists ta
UNION ALL
SELECT t.id, t.artist_library_id, NULL, NULL
FROM tracks t
WHERE t.artist_library_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM track_artists ta WHERE ta.track_id = t.id);

DROP VIEW album_credit_keys;
CREATE VIEW album_credit_keys AS
SELECT aa.album_id AS album_id,
       coalesce((SELECT r.library_id FROM artists r
                 WHERE r.library_id IS NOT NULL AND r.catalog_id = aa.artist_catalog_id AND r.name_key = aa.name_key
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

DROP VIEW track_credit_keys;
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

DROP INDEX artists_credit_key;
ALTER TABLE album_artists DROP COLUMN name_key;
ALTER TABLE track_artists DROP COLUMN name_key;
ALTER TABLE artists DROP COLUMN name_key;
