-- name: EnqueuePushTicket :exec
-- Queues a successfully-sent Expo ticket for a later receipt check — the
-- send response itself does not say whether the push was actually delivered.
INSERT INTO push_ticket_outbox (token, ticket_id) VALUES (sqlc.arg(token), sqlc.arg(ticket_id));

-- name: ClaimDuePushTickets :many
-- Tickets old enough for Expo to have an answer, oldest first. Read-only
-- (this queue has no lease/claim bookkeeping — see DeletePushTickets):
-- cmd/push-receipts is a run-once-and-exit cron worker, not a long-running
-- pool of overlapping consumers, so a plain scan is enough.
SELECT * FROM push_ticket_outbox
WHERE created_at <= now() - make_interval(mins => sqlc.arg(min_age_minutes)::int)
ORDER BY created_at
LIMIT sqlc.arg(batch_size);

-- name: DeletePushTickets :exec
-- Removes tickets once their receipt has been checked, regardless of outcome
-- (delivered, pruned-as-dead, or any other terminal receipt status) — this
-- queue has no retry bookkeeping of its own; an unprocessed batch (e.g. Expo
-- unreachable) simply stays queued for the next scheduled run.
DELETE FROM push_ticket_outbox WHERE id = ANY(sqlc.arg(ids)::bigint[]);
