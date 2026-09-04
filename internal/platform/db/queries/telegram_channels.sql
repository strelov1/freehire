-- name: ListActiveTelegramChannels :many
-- The channels cmd/tg-ingest crawls and cmd/tg-extract reads a kind from. Ordered by
-- name so a run's channel order is stable and its log diffable.
SELECT channel, kind FROM telegram_channels
WHERE active
ORDER BY channel;
