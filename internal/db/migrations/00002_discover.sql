-- Discover pages (Home recommendations, New, rooms, curators, groupings,
-- charts), cached as Apple's normalised responses. Home is personal
-- (personal = 1) and is wiped on sign-out; editorial pages are storefront
-- data and survive it. expires_at is Apple's nextUpdateDate for Home and a
-- fixed lifetime for editorial pages; expired entries are still served
-- until a refresh succeeds.

-- +goose Up
CREATE TABLE discover_cache (
    key        TEXT PRIMARY KEY,
    storefront TEXT    NOT NULL,
    personal   INTEGER NOT NULL DEFAULT 0,
    body       BLOB    NOT NULL,
    fetched_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX discover_cache_fetched ON discover_cache (fetched_at);

-- +goose Down
DROP TABLE discover_cache;
