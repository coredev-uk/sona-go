-- Recent searches remember whether a station had no page of its own (a local
-- broadcaster plays when it is chosen), so a remembered station does what the
-- live result does.

-- +goose Up
ALTER TABLE recent_searches ADD COLUMN no_page INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE recent_searches DROP COLUMN no_page;
