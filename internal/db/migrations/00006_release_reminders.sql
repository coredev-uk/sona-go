-- Release reminders: pre-release albums the user asked to be told about
-- when they come out (services/reminders.go). One desktop notification per
-- album at release_at, or at the next start after it; the row goes once it
-- has been shown. User data, cleared by WipeUserData on sign-out.

-- +goose Up
CREATE TABLE release_reminders (
    album_id         TEXT    PRIMARY KEY, -- catalog album ID
    name             TEXT    NOT NULL,
    artist_name      TEXT    NOT NULL DEFAULT '',
    artwork_template TEXT    NOT NULL DEFAULT '',
    -- When the album comes out: Unix ms, local midnight on its release date.
    release_at       INTEGER NOT NULL,
    -- The notification's body, in the interface's language when it was set.
    message          TEXT    NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL
);
CREATE INDEX release_reminders_release_at ON release_reminders (release_at);

-- +goose Down
DROP TABLE release_reminders;
