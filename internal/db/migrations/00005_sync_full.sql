-- When each library sync domain last completed a full run (every item
-- fetched and anything Apple no longer lists removed). Runs in between are
-- incremental (internal/library); a full run is due again after
-- library.FullSyncEvery. Every run before this migration was full.

-- +goose Up
ALTER TABLE sync_state ADD COLUMN last_full INTEGER;
UPDATE sync_state SET last_full = last_success;

-- +goose Down
ALTER TABLE sync_state DROP COLUMN last_full;
