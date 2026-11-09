-- Queries for the CV lifecycle letters (internal/engage/cvlifecycle). See migration 0248.
--
-- The eligibility predicate (a mailable address, a parsed CV, the free tier, consent, not
-- a holdout, the step due and its successor not) appears twice on purpose: in
-- ListCVLifecycleDue, which only DECIDES whom to prepare a letter for, and in
-- ClaimCVLifecycleSend, which re-checks it inside the statement that writes the step row.
-- The list may be stale by the time the runner reaches a user — somebody unsubscribed,
-- paid or lapsed — and only the claim's copy is the one that gates the send. Change one,
-- change both.

-- name: EnrollCVLifecycleUsers :execrows
-- Enrols accounts whose CV has just been parsed. One row per account, ever: a re-upload
-- never moves uploaded_at, so the clock the offsets run from is the first parsed upload
-- seen, and NOT EXISTS plus ON CONFLICT keep a concurrent run from adding a second.
-- Consent is not tested here: an account that had already opted out is enrolled and then
-- marked unsubscribed by ExitCVLifecycleUnsubscribed, so the conversion read still has it.
INSERT INTO cv_lifecycle_enrollments (user_id, uploaded_at, holdout)
SELECT u.id, u.resume_uploaded_at, cv_lifecycle_holdout(u.id)
FROM users u
WHERE mail_recipient(u.email, u.email_verified, u.last_active_at) <> ''
  AND cv_parsed(u.resume_extract_status, u.resume_structured_uploaded_at, u.resume_uploaded_at)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND u.resume_uploaded_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND NOT EXISTS (SELECT 1 FROM cv_lifecycle_enrollments e WHERE e.user_id = u.id)
ORDER BY u.resume_uploaded_at
LIMIT sqlc.arg(max_rows)::int
ON CONFLICT (user_id) DO NOTHING;

-- name: CountCVLifecycleEnrollable :one
-- What EnrollCVLifecycleUsers would enrol, for a dry run. Same predicate, no LIMIT.
SELECT count(*)
FROM users u
WHERE mail_recipient(u.email, u.email_verified, u.last_active_at) <> ''
  AND cv_parsed(u.resume_extract_status, u.resume_structured_uploaded_at, u.resume_uploaded_at)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND u.resume_uploaded_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND NOT EXISTS (SELECT 1 FROM cv_lifecycle_enrollments e WHERE e.user_id = u.id);

-- name: ObserveCVLifecyclePaid :execrows
-- The paying observation. An enrolment inside its window with no paid_at yet whose Stripe
-- or RevenueCat tier (Pro or Ultra) is live has STARTED PAYING: paid_at is stamped, status
-- becomes 'paid' and the sequence ends. Holdout and unsubscribed rows are observed too,
-- since the point is to compare them with the mailed ones. A granted tier is deliberately
-- not here: it is not a payment (see ObserveCVLifecycleEntitled).
UPDATE cv_lifecycle_enrollments e
SET status = 'paid',
    paid_at = now(),
    exited_at = COALESCE(e.exited_at, now())
FROM users u
WHERE u.id = e.user_id
  AND e.paid_at IS NULL
  AND e.enrolled_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND (u.pro_until_stripe > now()
       OR u.pro_until_revenuecat > now()
       OR u.ultra_until_stripe > now()
       OR u.ultra_until_revenuecat > now());

-- name: ObserveCVLifecycleEntitled :execrows
-- A live tier that is only GRANTED (by hand, or an invite reward) is not a payment: the
-- row is marked 'entitled' with no paid_at, so it is not counted as a conversion. Runs
-- after ObserveCVLifecyclePaid, so a row with both is already 'paid'. The row stays
-- eligible for letters when the grant lapses, which is why 'entitled' is not a final state.
UPDATE cv_lifecycle_enrollments e
SET status = 'entitled'
FROM users u
WHERE u.id = e.user_id
  AND e.status = 'active'
  AND e.paid_at IS NULL
  AND e.enrolled_at > now() - make_interval(days => sqlc.arg(window_days)::int)
  AND (u.pro_until_granted > now() OR u.ultra_until_granted > now());

-- name: ExitCVLifecycleUnsubscribed :execrows
-- Ends the sequence for good for an account that turned the letters off. The unsubscribe
-- link writes news_email_enabled = false without a session; a missing settings row is
-- "never touched it", not an opt-out, so only an explicit false counts.
UPDATE cv_lifecycle_enrollments e
SET status = 'unsubscribed',
    exited_at = COALESCE(e.exited_at, now())
FROM notification_settings ns
WHERE ns.user_id = e.user_id
  AND ns.news_email_enabled = false
  AND e.status IN ('active', 'entitled');

-- name: ListCVLifecycleDue :many
-- Accounts whose letter `step` is due and that no step row covers yet. Due means the
-- upload anchor plus `after_days` has passed; a step whose successor is also due
-- (`next_after_days`, NULL for the last step) is dropped, so a missed day never turns
-- into a burst of catch-up letters. An account that already holds today's daily claim is
-- left out, so a capped pass is not spent on people the claim would refuse.
-- Paged by user id (`after_user_id`, 0 for the first page): accounts the runner skips stay
-- due, and without a cursor they would fill the head of every page and starve the rest.
SELECT e.user_id, mail_recipient(u.email, u.email_verified, u.last_active_at)::text AS email
FROM cv_lifecycle_enrollments e
JOIN users u ON u.id = e.user_id
LEFT JOIN notification_settings ns ON ns.user_id = e.user_id
WHERE e.status IN ('active', 'entitled')
  AND NOT e.holdout
  AND mail_recipient(u.email, u.email_verified, u.last_active_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND cv_parsed(u.resume_extract_status, u.resume_structured_uploaded_at, u.resume_uploaded_at)
  AND e.uploaded_at + make_interval(days => sqlc.arg(after_days)::int) <= now()
  AND (sqlc.narg(next_after_days)::int IS NULL
       OR e.uploaded_at + make_interval(days => sqlc.narg(next_after_days)::int) > now())
  AND NOT EXISTS (
      SELECT 1 FROM cv_lifecycle_sends s WHERE s.user_id = e.user_id AND s.step = sqlc.arg(step)::smallint
  )
  AND NOT EXISTS (
      SELECT 1 FROM mail_day_claims c WHERE c.user_id = e.user_id AND c.day = mail_today()
  )
  AND e.user_id > sqlc.arg(after_user_id)::bigint
ORDER BY e.user_id
LIMIT sqlc.arg(max_rows)::int;

-- name: ClaimCVLifecycleSend :one
-- Writes the step row if, and only if, the account is STILL eligible: the one statement
-- that gates a send. It repeats ListCVLifecycleDue's predicate (see the note at the top),
-- so a CV re-uploaded and not yet parsed again, a new unsubscribe or a payment stops the
-- letter even when the list was read a moment earlier.
-- No row back (pgx.ErrNoRows) means do not send. The row is written before SES is called,
-- so a crash or a concurrent run can never send the same step twice.
INSERT INTO cv_lifecycle_sends (user_id, step)
SELECT e.user_id, sqlc.arg(step)::smallint
FROM cv_lifecycle_enrollments e
JOIN users u ON u.id = e.user_id
LEFT JOIN notification_settings ns ON ns.user_id = e.user_id
WHERE e.user_id = sqlc.arg(user_id)
  AND e.status IN ('active', 'entitled')
  AND NOT e.holdout
  AND mail_recipient(u.email, u.email_verified, u.last_active_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND cv_parsed(u.resume_extract_status, u.resume_structured_uploaded_at, u.resume_uploaded_at)
  AND e.uploaded_at + make_interval(days => sqlc.arg(after_days)::int) <= now()
  AND (sqlc.narg(next_after_days)::int IS NULL
       OR e.uploaded_at + make_interval(days => sqlc.narg(next_after_days)::int) > now())
ON CONFLICT (user_id, step) DO NOTHING
RETURNING user_id;

-- name: RecordCVLifecycleSendError :exec
-- Burns the step after a definite send failure: the row stays, with the error, and the
-- step is never retried on its own — re-arming one is a deliberate DELETE.
UPDATE cv_lifecycle_sends
SET error = sqlc.arg(error)::text
WHERE user_id = sqlc.arg(user_id) AND step = sqlc.arg(step)::smallint;

-- name: ReleaseCVLifecycleSend :exec
-- Takes the step row back when the letter was not sent for a reason that is not the
-- account's fault (the daily claim was lost, or sending is paused), so the step stays
-- pending. Only an unerrored row: a burned step is never reopened by this.
DELETE FROM cv_lifecycle_sends
WHERE user_id = sqlc.arg(user_id) AND step = sqlc.arg(step)::smallint AND error = '';

-- The paywall letter (step 4): one letter to a free user who was refused by the paid-AI gate,
-- about an hour after the first refusal that is still fresh. It shares the CV letters'
-- ledger (cv_lifecycle_sends), consent switch, free-tier test and holdout, but it is not
-- gated on an enrolment: the account may never have uploaded a CV. The holdout is the same
-- hash enrolment stores, so an account is in the same group for every letter. The predicate
-- appears in the list and in the claim, as above: change one, change both.

-- name: ListPaywallLetterDue :many
-- Accounts the paywall letter is due for, with the earliest hit inside the window to name:
-- at least hold_minutes old (the letter answers the attempt a while after, not mid-click) and
-- at most max_age_hours old (an old hit is no longer "what you just tried"). One row per
-- account, whatever the number of hits, and none once the step row exists: the letter is
-- once per account, never once per hit. The job, when the hit carries one, is read here so
-- the letter is composed from the row; a job that is gone, or a private one the account did
-- not create, leaves the strings empty.
-- Paged by user id like ListCVLifecycleDue.
SELECT DISTINCT ON (h.user_id)
    h.user_id,
    mail_recipient(u.email, u.email_verified, u.last_active_at)::text AS email,
    h.feature,
    COALESCE(j.title, '')::text AS job_title,
    COALESCE(j.company, '')::text AS job_company,
    COALESCE(j.public_slug, '')::text AS job_slug
FROM paywall_hits h
JOIN users u ON u.id = h.user_id
LEFT JOIN jobs j ON j.id = h.job_id AND (NOT j.is_private OR j.created_by = h.user_id)
LEFT JOIN notification_settings ns ON ns.user_id = h.user_id
WHERE h.created_at <= now() - make_interval(mins => sqlc.arg(hold_minutes)::int)
  AND h.created_at > now() - make_interval(hours => sqlc.arg(max_age_hours)::int)
  AND NOT cv_lifecycle_holdout(h.user_id)
  AND mail_recipient(u.email, u.email_verified, u.last_active_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND NOT EXISTS (
      SELECT 1 FROM cv_lifecycle_sends s WHERE s.user_id = h.user_id AND s.step = 4
  )
  AND NOT EXISTS (
      SELECT 1 FROM mail_day_claims c WHERE c.user_id = h.user_id AND c.day = mail_today()
  )
  AND h.user_id > sqlc.arg(after_user_id)::bigint
ORDER BY h.user_id, h.created_at, h.id
LIMIT sqlc.arg(max_rows)::int;

-- name: ClaimPaywallLetterSend :one
-- Writes the step-4 row if, and only if, the account is STILL eligible and still has a hit
-- inside the window: the one statement that gates the send. No row back (pgx.ErrNoRows)
-- means do not send. Written before SES is called, like ClaimCVLifecycleSend.
INSERT INTO cv_lifecycle_sends (user_id, step)
SELECT u.id, 4
FROM users u
LEFT JOIN notification_settings ns ON ns.user_id = u.id
WHERE u.id = sqlc.arg(user_id)
  AND NOT cv_lifecycle_holdout(u.id)
  AND mail_recipient(u.email, u.email_verified, u.last_active_at) <> ''
  AND COALESCE(ns.news_email_enabled, true)
  AND (u.pro_until IS NULL OR u.pro_until <= now())
  AND (u.ultra_until IS NULL OR u.ultra_until <= now())
  AND EXISTS (
      SELECT 1 FROM paywall_hits h
      WHERE h.user_id = u.id
        AND h.created_at <= now() - make_interval(mins => sqlc.arg(hold_minutes)::int)
        AND h.created_at > now() - make_interval(hours => sqlc.arg(max_age_hours)::int)
  )
ON CONFLICT (user_id, step) DO NOTHING
RETURNING user_id;
