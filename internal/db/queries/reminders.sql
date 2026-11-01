-- Release reminders (services/reminders.go).

-- name: ListReminders :many
SELECT * FROM release_reminders ORDER BY release_at, name, album_id;

-- name: PutReminder :exec
INSERT INTO release_reminders (album_id, name, artist_name, artwork_template, release_at, message, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (album_id) DO UPDATE SET
    name = excluded.name,
    artist_name = excluded.artist_name,
    artwork_template = excluded.artwork_template,
    release_at = excluded.release_at,
    message = excluded.message;

-- name: SetReminderRelease :exec
UPDATE release_reminders SET release_at = ? WHERE album_id = ?;

-- name: DeleteReminder :execrows
DELETE FROM release_reminders WHERE album_id = ?;

-- name: ClearReminders :exec
DELETE FROM release_reminders;
