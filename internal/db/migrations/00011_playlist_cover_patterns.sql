-- A 'preset' playlist cover is now one of the designs Apple draws a
-- playlist's generated cover in, its template the design ('PDCXS14'; see
-- internal/artwork/pattern.go). The covers Sona drew itself ('preset:…'
-- templates, 0.3.0 release candidates) are gone: such a choice goes back to
-- the automatic cover, keeping the image the user chose, if any.

-- +goose Up
DELETE FROM playlist_covers WHERE kind = 'preset' AND upload = '';
UPDATE playlist_covers SET kind = 'auto', template = '' WHERE kind = 'preset';

-- +goose Down
DELETE FROM playlist_covers WHERE kind = 'preset' AND upload = '';
UPDATE playlist_covers SET kind = 'auto', template = '' WHERE kind = 'preset';
