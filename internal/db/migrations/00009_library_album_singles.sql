-- Library albums carry Apple's single/EP flag (albums.is_single): a library
-- album's catalog album is fetched with isSingle, and a name ending in
-- " - Single" or " - EP" counts too (library.IsSingleRelease). Albums stored
-- before this have only the name to go on, so the suffix sets the flag for
-- them now, and clearing the albums domain's last full run makes the next
-- sync a full one, which stores the catalog's flag for every library album.

-- +goose Up
UPDATE albums SET is_single = 1
WHERE library_id IS NOT NULL AND (name GLOB '* - Single' OR name GLOB '* - EP');
UPDATE sync_state SET last_full = NULL WHERE domain = 'albums';

-- +goose Down
UPDATE albums SET is_single = 0 WHERE library_id IS NOT NULL;
