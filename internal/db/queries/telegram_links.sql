-- name: UpsertTelegramLink :exec
-- Link (or relink) a user's Telegram chat, captured from the inbound /start. One
-- row per user; relinking from a different chat overwrites the chat_id.
INSERT INTO telegram_links (user_id, chat_id)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET chat_id = EXCLUDED.chat_id, linked_at = now();

-- name: GetTelegramLink :one
-- The caller's linked Telegram chat (link-status endpoint + delivery resolution).
SELECT * FROM telegram_links WHERE user_id = $1;

-- name: GetUserIDByTelegramChat :one
-- Reverse lookup: the user linked to an inbound chat, for contribution-from-Telegram. If a
-- chat somehow linked more than once, the most recently linked user wins.
SELECT user_id FROM telegram_links WHERE chat_id = $1 ORDER BY linked_at DESC LIMIT 1;

-- name: DeleteTelegramLink :execrows
-- Unlink Telegram. Returns the affected row count: 0 means there was no link.
DELETE FROM telegram_links WHERE user_id = $1;
