-- Queries behind the CV lifecycle letters report (internal/engage/cvlifecycle, cmd/report-cv-letters).
-- Read-only. "Paid" is cv_lifecycle_enrollments.paid_at, the first time the paying
-- observation saw a Stripe or RevenueCat tier live (see ObserveCVLifecyclePaid).

-- name: CVLetterEngagement :many
-- What SES reported for the letters' campaigns, per campaign and event type. email_events
-- holds the last 90 days and email_event_daily what was rolled up out of it; a row lives in
-- exactly one of them (RollUpExpiredEmailEvents moves it), so the two are added, not merged.
SELECT campaign::text AS campaign, type::text AS type, sum(n)::bigint AS events
FROM (
    SELECT campaign, type, count(*) AS n
    FROM email_events
    WHERE campaign LIKE 'cv-letter-%'
    GROUP BY campaign, type
    UNION ALL
    SELECT campaign, type, sum(count)
    FROM email_event_daily
    WHERE campaign LIKE 'cv-letter-%'
    GROUP BY campaign, type
) t
GROUP BY campaign, type
ORDER BY campaign, type;

-- name: CVLetterPayments :one
-- One letter's recipients and how many of them started paying within `window_days` of it.
-- `sent` is every letter the ledger holds for the step; `matured` and `paid` count only
-- letters at least `window_days` old, so a recipient whose window is still open is not
-- counted as a non-payer. An error-free step row is a sent letter; the rare crash that kept
-- an empty-errored row without sending is counted as sent, which can only lower the rate.
SELECT
    count(*)::bigint AS sent,
    count(*) FILTER (WHERE s.sent_at <= now() - make_interval(days => sqlc.arg(window_days)::int))::bigint AS matured,
    count(*) FILTER (
        WHERE s.sent_at <= now() - make_interval(days => sqlc.arg(window_days)::int)
          AND e.paid_at >= s.sent_at
          AND e.paid_at <= s.sent_at + make_interval(days => sqlc.arg(window_days)::int)
    )::bigint AS paid
FROM cv_lifecycle_sends s
JOIN cv_lifecycle_enrollments e ON e.user_id = s.user_id
WHERE s.step = sqlc.arg(step)::smallint
  AND s.error = '';

-- name: CVLetterHoldoutAtStep :one
-- The control for one letter, on the same clock: holdout accounts that had not paid when the
-- letter fell due (upload + `after_days`) and whose window of `window_days` from that moment
-- has closed, and how many of them started paying inside it. Letter 3 goes to people who did
-- not pay in the first 13 days, so setting its recipients against a rate counted from
-- enrollment would compare different weeks of an account's life.
SELECT
    count(*)::bigint AS accounts,
    count(*) FILTER (
        WHERE e.paid_at IS NOT NULL
          AND e.paid_at <= e.uploaded_at + make_interval(days => sqlc.arg(after_days)::int + sqlc.arg(window_days)::int)
    )::bigint AS paid
FROM cv_lifecycle_enrollments e
WHERE e.holdout
  AND e.uploaded_at + make_interval(days => sqlc.arg(after_days)::int + sqlc.arg(window_days)::int) <= now()
  AND (e.paid_at IS NULL OR e.paid_at > e.uploaded_at + make_interval(days => sqlc.arg(after_days)::int));

-- name: CVLetterGroupsFromEnrollment :many
-- Everyone enrolled, by group, and how many started paying within `window_days` of being
-- enrolled; rows still inside their window are left out. `holdout = false` is the whole mailed
-- group whether or not a letter reached the account (unsubscribed, skipped and paid-before-
-- the-letter accounts stay in), which is the comparison that cannot be flattered.
SELECT
    e.holdout,
    count(*)::bigint AS accounts,
    count(*) FILTER (
        WHERE e.paid_at IS NOT NULL
          AND e.paid_at <= e.enrolled_at + make_interval(days => sqlc.arg(window_days)::int)
    )::bigint AS paid
FROM cv_lifecycle_enrollments e
WHERE e.enrolled_at <= now() - make_interval(days => sqlc.arg(window_days)::int)
GROUP BY e.holdout
ORDER BY e.holdout;
