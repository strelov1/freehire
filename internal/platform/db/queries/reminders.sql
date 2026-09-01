-- name: GetNotificationSettings :one
-- The caller's notification rule, shared by saved-job reminders and both
-- lifecycle nudges. No row -> pgx.ErrNoRows, which the service reads as the
-- opt-out-by-default state (never configured; see the
-- centralize-lifecycle-notifications change for why the default is enabled).
SELECT * FROM notification_settings WHERE user_id = $1;

-- name: UpsertNotificationSettings :one
-- Create or replace the caller's notification rule in one statement, including the
-- delivery-timing fields (digest frequency/time, quiet hours) alongside the
-- enable/channels gate — one account-level settings row, one write path. Returns
-- the stored row.
INSERT INTO notification_settings (
    user_id, enabled, channels, digest_frequency, digest_time,
    quiet_hours_start, quiet_hours_end, updated_at
)
VALUES (
    sqlc.arg(user_id), sqlc.arg(enabled), sqlc.arg(channels), sqlc.arg(digest_frequency),
    sqlc.arg(digest_time), sqlc.arg(quiet_hours_start), sqlc.arg(quiet_hours_end), now()
)
ON CONFLICT (user_id) DO UPDATE
  SET enabled            = EXCLUDED.enabled,
      channels           = EXCLUDED.channels,
      digest_frequency   = EXCLUDED.digest_frequency,
      digest_time        = EXCLUDED.digest_time,
      quiet_hours_start  = EXCLUDED.quiet_hours_start,
      quiet_hours_end    = EXCLUDED.quiet_hours_end,
      updated_at         = now()
RETURNING *;

-- name: GetReminderScheduleContext :one
-- The two halves of "when should this account hear from us": the daily
-- notification hour (notification_settings.digest_time) and the timezone to read
-- it in (users.timezone). Both are optional and both are read here rather than
-- from GetNotificationSettings, which knows nothing about the user row — one
-- statement so the hour and the zone can never come from different reads and
-- disagree. The row always exists for a real user; a NULL in either column is the
-- unconfigured state the caller resolves to its own defaults.
SELECT ns.digest_time AS digest_time,
       u.timezone     AS timezone
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.id = $1;

-- name: UpsertJobReminder :one
-- Schedule a one-shot reminder for a saved job, or replace the pending one if the
-- job is re-saved with a new choice. The arbiter is the partial unique index on
-- (user_id, job_id) WHERE status='pending', so only a live pending reminder is
-- replaced; delivered/cancelled history rows never conflict. The conflict path
-- resets the delivery ledger (claimed_at/attempts/last_error) since the schedule
-- changed. Returns the pending row.
INSERT INTO job_reminders (user_id, job_id, fire_at, channels)
VALUES (sqlc.arg(user_id), sqlc.arg(job_id), sqlc.arg(fire_at), sqlc.arg(channels)::text[])
ON CONFLICT (user_id, job_id) WHERE status = 'pending'
DO UPDATE SET fire_at    = EXCLUDED.fire_at,
             channels   = EXCLUDED.channels,
             claimed_at = NULL,
             attempts   = 0,
             last_error = ''
RETURNING *;

-- name: CancelJobReminder :execrows
-- Cancel the pending reminder for one (user, job): the eager cleanup wired into
-- apply and unsave (there is no per-job manual control any more — the shared
-- notification_settings toggle is the only control). Idempotent — no pending row
-- affects 0 rows and is never an error. Cancelled rows are retained as history.
UPDATE job_reminders
SET status = 'cancelled'
WHERE user_id = $1 AND job_id = $2 AND status = 'pending';

-- name: ClaimDueReminders :many
-- Lease a batch of due, pending reminders by stamping claimed_at, earliest deadline
-- first. FOR UPDATE OF r + SKIP LOCKED lets overlapping worker passes take disjoint
-- rows so a reminder fires at most once; the lease predicate reclaims rows whose
-- sender died (stale claimed_at), so no separate reaper is needed. Delivery happens
-- OUTSIDE this transaction, so no network call is held under a row lock.
WITH claimable AS (
    SELECT r.id
    FROM job_reminders r
    WHERE r.status = 'pending'
      AND r.failed_at IS NULL
      AND r.fire_at <= now()
      AND (r.claimed_at IS NULL
           OR r.claimed_at < now() - make_interval(secs => sqlc.arg(lease_seconds)::int))
    ORDER BY r.fire_at, r.id
    FOR UPDATE OF r SKIP LOCKED
    LIMIT sqlc.arg(batch_size)
), claimed AS (
    UPDATE job_reminders r
    SET claimed_at = now()
    FROM claimable c
    WHERE r.id = c.id
    RETURNING r.id, r.fire_at
)
-- Sorted OUTSIDE the UPDATE. The CTE's ORDER BY only picks WHICH rows are claimed;
-- an UPDATE ... RETURNING is not obliged to emit them in that order, and the engine
-- groups the result into one message per account, listing the jobs in the order it
-- receives them. Without this the list order is whatever the join produced.
SELECT id FROM claimed ORDER BY fire_at, id;

-- name: GetReminderForDelivery :one
-- The delivery context for one reminder: the job display fields, the channel set,
-- the user's live destinations (account email; linked Telegram chat, NULL when
-- unlinked -> that channel soft-skips; whether a push device is registered), the
-- fire-time re-check flags, and the live quiet-hours window (account timezone +
-- notification_settings' quiet_hours_start/end) that internal/application/deliverywindow
-- checks before delivery. job_open and still_actionable let the worker
-- cancel-and-skip a reminder whose job has since closed or is no longer
-- saved-but-unapplied, closing the race between a cancel and the fire.
SELECT r.id, r.user_id, r.job_id, r.channels,
       j.title, j.company, j.public_slug, j.url,
       (j.closed_at IS NULL)::bool AS job_open,
       COALESCE(uj.saved_at IS NOT NULL AND a.applied_at IS NULL, false)::bool AS still_actionable,
       u.email AS account_email,
       u.timezone AS timezone,
       tl.chat_id AS telegram_chat_id,
       EXISTS(SELECT 1 FROM user_push_tokens upt WHERE upt.user_id = r.user_id) AS has_push_device,
       ns.quiet_hours_start AS quiet_hours_start,
       ns.quiet_hours_end AS quiet_hours_end
FROM job_reminders r
JOIN jobs j ON j.id = r.job_id
JOIN users u ON u.id = r.user_id
LEFT JOIN user_jobs uj ON uj.user_id = r.user_id AND uj.job_id = r.job_id
LEFT JOIN applications a ON a.user_id = r.user_id AND a.job_id = r.job_id
LEFT JOIN telegram_links tl ON tl.user_id = r.user_id
LEFT JOIN notification_settings ns ON ns.user_id = r.user_id
WHERE r.id = $1;

-- name: MarkReminderDelivered :execrows
-- Terminal success: flip a fired reminder to delivered so it leaves the pending
-- scan and is never sent again. Guarded on status='pending' for idempotency under
-- a worker retry that already delivered.
UPDATE job_reminders
SET status = 'delivered', delivered_at = now()
WHERE id = $1 AND status = 'pending';

-- name: CancelReminderAtFire :execrows
-- Lazy cancellation at fire time: the worker's re-check found the job closed or no
-- longer saved-but-unapplied, so cancel instead of sending. This is how job closure
-- cancels reminders without hooking every scattered close path.
UPDATE job_reminders
SET status = 'cancelled'
WHERE id = $1 AND status = 'pending';

-- name: RecordReminderDeliveryFailure :exec
-- Count a failed send: bump attempts, record the error, and dead-letter (failed_at)
-- once attempts reach the max. claimed_at is left in place — its expiry gates the
-- retry to a later pass and doubles as the crash reaper, mirroring subscription_matches.
UPDATE job_reminders
SET attempts   = attempts + 1,
    last_error = sqlc.arg(last_error),
    failed_at  = CASE
                     WHEN attempts + 1 >= sqlc.arg(max_attempts)::int THEN now()
                     ELSE NULL
                 END
WHERE id = sqlc.arg(id);

-- name: ReleaseReminderClaim :exec
-- Release the lease without counting an attempt, so a soft-skipped send (e.g. no
-- usable destination on any configured channel) is retried promptly on a later pass
-- instead of waiting out the lease.
UPDATE job_reminders
SET claimed_at = NULL
WHERE id = $1;
