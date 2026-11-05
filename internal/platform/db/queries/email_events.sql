-- Queries behind the engagement ledger (internal/engage/emailevents, cmd/email-events).

-- name: RecordEmailEvent :exec
-- First sighting wins. SQS delivers at least once and a person can open a letter ten times;
-- neither should change a rate.
INSERT INTO email_events (message_id, type, campaign, variant, occurred_at)
VALUES (sqlc.arg(message_id), sqlc.arg(type), sqlc.arg(campaign), sqlc.arg(variant), sqlc.arg(occurred_at))
ON CONFLICT (message_id, type) DO NOTHING;

-- name: RollUpExpiredEmailEvents :execrows
-- One bounded batch of events past retention, moved into the daily counts in a single
-- statement, so a crash can neither lose a count nor add it twice.
WITH expired AS (
    DELETE FROM email_events
    WHERE (message_id, type) IN (
        SELECT e.message_id, e.type FROM email_events e
        WHERE e.occurred_at < sqlc.arg(before)
        ORDER BY e.occurred_at
        LIMIT sqlc.arg(max_rows)::int
    )
    RETURNING occurred_at, campaign, variant, type
)
INSERT INTO email_event_daily (day, campaign, variant, type, count)
SELECT (occurred_at AT TIME ZONE 'UTC')::date, campaign, variant, type, count(*)
FROM expired
GROUP BY 1, 2, 3, 4
ON CONFLICT (day, campaign, variant, type) DO UPDATE
SET count = email_event_daily.count + EXCLUDED.count;
