-- Recent searches: the terms the user searched and the results they opened
-- from search, newest first. Apple keeps these client-side only (no API);
-- they are user data, cleared by WipeUserData on sign-out.

-- +goose Up
CREATE TABLE recent_searches (
    -- "term:<scope>:<lowercased term>" or "item:<kind>:<source>:<id>": one row per search or item.
    key         TEXT    PRIMARY KEY,
    -- "term", or the opened item's kind ("song", "album", "artist", "playlist", "station", …).
    kind        TEXT    NOT NULL,
    -- "catalog" or "library": the scope a term was searched in, or the item's ID source.
    scope       TEXT    NOT NULL,
    term        TEXT    NOT NULL DEFAULT '',
    item_id     TEXT    NOT NULL DEFAULT '',
    name        TEXT    NOT NULL DEFAULT '',
    subtitle    TEXT    NOT NULL DEFAULT '',
    -- Go-served /art/ URL of the item's artwork as shown in the results ('' for terms).
    art_url     TEXT    NOT NULL DEFAULT '',
    searched_at INTEGER NOT NULL
);
CREATE INDEX recent_searches_searched_at ON recent_searches (searched_at DESC);

-- +goose Down
DROP TABLE recent_searches;
