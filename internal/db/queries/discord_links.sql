-- name: UpsertDiscordLink :exec
-- Link (or relink) a user's Discord account, captured from the inbound /link command. One
-- row per user; relinking from a different Discord account overwrites the discord_id.
INSERT INTO discord_links (user_id, discord_id)
VALUES ($1, $2)
ON CONFLICT (user_id) DO UPDATE
SET discord_id = EXCLUDED.discord_id, linked_at = now();

-- name: GetDiscordLink :one
-- The caller's linked Discord account (link-status endpoint + delivery resolution).
SELECT * FROM discord_links WHERE user_id = $1;

-- name: GetUserIDByDiscordID :one
-- Reverse lookup: the user linked to an inbound Discord account, for contribution-from-Discord. If a
-- Discord account somehow linked more than once, the most recently linked user wins.
SELECT user_id FROM discord_links WHERE discord_id = $1 ORDER BY linked_at DESC LIMIT 1;

-- name: DeleteDiscordLink :execrows
-- Unlink Discord. Returns the affected row count: 0 means there was no link.
DELETE FROM discord_links WHERE user_id = $1;
