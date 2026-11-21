-- The notification centre (services/notifications.go).

-- name: ListNotifications :many
SELECT * FROM notifications ORDER BY created_at DESC, id DESC LIMIT 100;

-- name: PutNotification :exec
INSERT INTO notifications (kind, key, subject, artist, variant, count, artwork_template, target_kind, target_id, account, created_at, read)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)
ON CONFLICT (key) DO UPDATE SET
    kind = excluded.kind,
    subject = excluded.subject,
    artist = excluded.artist,
    variant = excluded.variant,
    count = excluded.count,
    artwork_template = excluded.artwork_template,
    target_kind = excluded.target_kind,
    target_id = excluded.target_id,
    account = excluded.account,
    created_at = excluded.created_at,
    read = 0;

-- name: MarkNotificationsRead :execrows
UPDATE notifications SET read = 1 WHERE read = 0;

-- name: DeleteNotification :execrows
DELETE FROM notifications WHERE id = ?;

-- name: ClearNotifications :execrows
DELETE FROM notifications;

-- name: ClearAccountNotifications :execrows
DELETE FROM notifications WHERE account = 1;

-- name: PruneNotifications :exec
DELETE FROM notifications
WHERE notifications.created_at < sqlc.arg(before)
   OR notifications.id NOT IN (SELECT n.id FROM notifications AS n ORDER BY n.created_at DESC, n.id DESC LIMIT 100);
