-- The storefront's content languages (supportedLanguageTags from
-- /v1/me/storefront), space-separated BCP 47 tags. Apple requests ask for
-- the user's language (l=) only when the storefront offers it
-- (applemusic.ContentLanguage). Filled at the next token validation.

-- +goose Up
ALTER TABLE accounts ADD COLUMN languages TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE accounts DROP COLUMN languages;
