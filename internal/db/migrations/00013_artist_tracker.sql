-- The artist tracker (services/artisttracker.go): new releases from the
-- artists the user has favourited, found by a daily catalogue check once the
-- setting is on. All of it is user data, cleared by WipeUserData on sign-out.
--
-- tracked_artists are the favourite artists whose releases at the time were
-- recorded as seen (their first check announces nothing); an artist that
-- isn't in it yet is seeded on the next check. Turning the setting on
-- empties it, so the releases that came out while it was off are never
-- announced.
--
-- artist_releases are the releases the tracker has seen, kept so that none is
-- announced twice, across checks and restarts. listed is 0 for a release
-- recorded as already out when its artist was seeded (or older than the
-- tracker looks back): seen, never shown. found_at is when the tracker saw
-- it; read_at and dismissed_at are the user's, NULL until then. A dismissed
-- row stays so the release isn't found again.
--
-- artist_tracker_state is one row: when the last complete check finished
-- (Unix ms), so a restart doesn't check again within the day.

-- +goose Up
CREATE TABLE tracked_artists (
    artist_id TEXT    PRIMARY KEY, -- catalog artist ID
    name      TEXT    NOT NULL DEFAULT '',
    seeded_at INTEGER NOT NULL
);

CREATE TABLE artist_releases (
    album_id         TEXT    PRIMARY KEY, -- catalog album ID
    artist_id        TEXT    NOT NULL,    -- catalog artist ID of the tracked artist
    name             TEXT    NOT NULL,
    artist_name      TEXT    NOT NULL DEFAULT '',
    artwork_template TEXT    NOT NULL DEFAULT '',
    kind             TEXT    NOT NULL CHECK (kind IN ('album', 'ep', 'single')),
    -- Apple's releaseDate (YYYY-MM-DD, or a year alone) and, for a full date,
    -- local midnight on it in Unix ms (0 otherwise).
    release_date     TEXT    NOT NULL DEFAULT '',
    release_at       INTEGER NOT NULL DEFAULT 0,
    -- Apple's isPrerelease at the last check.
    prerelease       INTEGER NOT NULL DEFAULT 0,
    track_count      INTEGER NOT NULL DEFAULT 0,
    explicit         INTEGER NOT NULL DEFAULT 0,
    listed           INTEGER NOT NULL DEFAULT 1,
    found_at         INTEGER NOT NULL,
    read_at          INTEGER,
    dismissed_at     INTEGER
);
CREATE INDEX artist_releases_artist ON artist_releases (artist_id);
CREATE INDEX artist_releases_found ON artist_releases (found_at);

CREATE TABLE artist_tracker_state (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    checked_at INTEGER NOT NULL
);

-- +goose Down
DROP TABLE artist_tracker_state;
DROP TABLE artist_releases;
DROP TABLE tracked_artists;
