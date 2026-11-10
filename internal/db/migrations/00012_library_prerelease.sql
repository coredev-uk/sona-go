-- A pre-added (unreleased) library album. Apple's library album carries no
-- release state of its own: a pre-added one has no releaseDate, and only its
-- catalogue album says when it comes out and whether it is a pre-release
-- (isPrerelease). The library walk now asks for both, and stores them here:
-- is_prerelease is Apple's flag on the catalogue album, catalog_release_date
-- the catalogue album's releaseDate (release_date stays the library's).
--
-- A pre-added album's songs are library songs too, but only those already out
-- carry playParams (and a releaseDate): has_play_params is 0 for the rest.
-- Rows stored before this are taken as playable and as released until the
-- full run that clearing the albums and songs domains' last full run makes
-- the next sync into stores the real values (as 00009 did).

-- +goose Up
ALTER TABLE albums ADD COLUMN is_prerelease INTEGER NOT NULL DEFAULT 0;
ALTER TABLE albums ADD COLUMN catalog_release_date TEXT NOT NULL DEFAULT '';
ALTER TABLE tracks ADD COLUMN has_play_params INTEGER NOT NULL DEFAULT 1;
UPDATE sync_state SET last_full = NULL WHERE domain IN ('albums', 'songs');

-- +goose Down
ALTER TABLE tracks DROP COLUMN has_play_params;
ALTER TABLE albums DROP COLUMN catalog_release_date;
ALTER TABLE albums DROP COLUMN is_prerelease;
