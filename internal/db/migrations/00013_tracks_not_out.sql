-- Songs not out yet (internal/library/release.go) are found as a query runs,
-- by the clock: a song is not out only if Apple gave it no playParams
-- (no_play_params = 1) or its own releaseDate is still ahead. These indexes
-- let the library count them, and leave them out of the song lists, without
-- reading every song. No data changes.

-- +goose Up
CREATE INDEX tracks_no_play_params ON tracks (no_play_params) WHERE no_play_params = 1;
CREATE INDEX tracks_release_date ON tracks (release_date);

-- +goose Down
DROP INDEX tracks_release_date;
DROP INDEX tracks_no_play_params;
