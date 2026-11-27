-- The name the signed-in person has on their Apple Music profile
-- (GET /v1/me/social/profile), kept so the For You greeting can use it on a
-- cold start with no request. profile_name is "" when there is no profile
-- or it is not set up (Apple Music asks people to set one up); the handle is
-- never stored. profile_checked_at is when Apple was last asked (Unix ms),
-- NULL when it never was: the name is renewed at sign-in and at most once a
-- day. Both live on the account row, so sign-out clears them with it.

-- +goose Up
ALTER TABLE accounts ADD COLUMN profile_name TEXT NOT NULL DEFAULT '';
ALTER TABLE accounts ADD COLUMN profile_checked_at INTEGER;

-- +goose Down
ALTER TABLE accounts DROP COLUMN profile_checked_at;
ALTER TABLE accounts DROP COLUMN profile_name;
