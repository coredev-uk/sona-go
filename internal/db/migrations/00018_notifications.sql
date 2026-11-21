-- The notification centre (services/notifications.go): what happened while
-- the person wasn't asking, kept until they dismiss it. A release they asked
-- to be reminded of came out, the artist tracker found new releases, an
-- add-on was switched off. The words are the UI's: a row keeps the kind and
-- the names, never a sentence. Rows about the account (account = 1) are user
-- data, cleared by sign-out; the rest stay. Pruned to the last 30 days and
-- at most 100 rows on every insert.

-- +goose Up
CREATE TABLE notifications (
    id               INTEGER PRIMARY KEY,
    kind             TEXT    NOT NULL,
    -- key names what the row is about ("release-out:<album>"): a second
    -- notice with the same key replaces the first and is unread again.
    key              TEXT    NOT NULL UNIQUE,
    subject          TEXT    NOT NULL DEFAULT '',
    artist           TEXT    NOT NULL DEFAULT '',
    variant          TEXT    NOT NULL DEFAULT '',
    count            INTEGER NOT NULL DEFAULT 0,
    artwork_template TEXT    NOT NULL DEFAULT '',
    target_kind      TEXT    NOT NULL DEFAULT '',
    target_id        TEXT    NOT NULL DEFAULT '',
    account          INTEGER NOT NULL DEFAULT 0,
    created_at       INTEGER NOT NULL, -- Unix ms
    read             INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX notifications_created_at ON notifications (created_at);

-- +goose Down
DROP TABLE notifications;
