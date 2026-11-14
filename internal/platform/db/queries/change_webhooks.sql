-- name: CreateChangeWebhook :one
-- A new subscription starts at the oldest transaction still in flight (the same bound the
-- feed serves below), so it delivers changes made from now on, none of the history.
INSERT INTO change_webhooks (user_id, url, secret, company_slugs, kinds, cursor_xid)
VALUES (
    sqlc.arg(user_id), sqlc.arg(url), sqlc.arg(secret), sqlc.arg(company_slugs)::text[], sqlc.arg(kinds)::text[],
    pg_snapshot_xmin(pg_current_snapshot())::text::bigint
)
RETURNING id, user_id, url, company_slugs, kinds, consecutive_failures, last_success_at, last_error,
          disabled_at, disabled_reason, created_at;

-- name: ListChangeWebhooks :many
SELECT id, user_id, url, company_slugs, kinds, consecutive_failures, last_success_at, last_error,
       disabled_at, disabled_reason, created_at
FROM change_webhooks
WHERE user_id = $1
ORDER BY id;

-- name: GetChangeWebhook :one
-- Scoped to its owner, like every account-owned row.
SELECT id, user_id, url, secret, company_slugs, kinds, cursor_xid, cursor_id,
       consecutive_failures, next_attempt_at, last_success_at, last_error, disabled_at, disabled_reason, created_at
FROM change_webhooks
WHERE id = $1 AND user_id = $2;

-- name: CountChangeWebhooks :one
SELECT count(*) FROM change_webhooks WHERE user_id = $1;

-- name: DeleteChangeWebhook :execrows
DELETE FROM change_webhooks WHERE id = $1 AND user_id = $2;

-- name: ClaimDueChangeWebhooks :many
-- The worker's pick: enabled subscriptions that are due, leased for lease_secs so a second
-- run does not deliver the same ones, and only for accounts with a live API plan. SKIP LOCKED keeps concurrent claimers apart. due_at is the run's start, so a row this run just delivered (next_attempt_at = now()) is not claimed again by it.
UPDATE change_webhooks w
SET next_attempt_at = now() + make_interval(secs => sqlc.arg(lease_secs)::bigint)
WHERE w.id IN (
    SELECT d.id FROM change_webhooks d
    WHERE d.disabled_at IS NULL AND d.next_attempt_at <= sqlc.arg(due_at)::timestamptz
      -- Only while the account holds an API plan (billing.APITierOf, in SQL): a lapsed one pauses.
      AND EXISTS (SELECT 1 FROM users u WHERE u.id = d.user_id
                  AND GREATEST(u.api_until_stripe, u.api_data_until_stripe) > now())
    ORDER BY d.next_attempt_at
    LIMIT sqlc.arg(row_limit)
    FOR UPDATE SKIP LOCKED
)
RETURNING w.id, w.user_id, w.url, w.secret, w.company_slugs, w.kinds, w.cursor_xid, w.cursor_id,
          w.consecutive_failures;

-- name: AdvanceChangeWebhook :exec
-- A batch was delivered (or nothing in the scanned range matched): move the position, clear
-- the failure state and release the lease.
UPDATE change_webhooks
SET cursor_xid = sqlc.arg(cursor_xid), cursor_id = sqlc.arg(cursor_id),
    consecutive_failures = 0, last_error = '', next_attempt_at = now(),
    last_success_at = CASE WHEN sqlc.arg(delivered)::boolean THEN now() ELSE last_success_at END
WHERE id = sqlc.arg(id);

-- name: FailChangeWebhook :exec
-- A delivery failed: count it and schedule the retry. The caller decides when to give up.
UPDATE change_webhooks
SET consecutive_failures = consecutive_failures + 1, last_error = sqlc.arg(last_error),
    next_attempt_at = now() + make_interval(secs => sqlc.arg(backoff_secs)::bigint)
WHERE id = sqlc.arg(id);

-- name: DisableChangeWebhook :exec
UPDATE change_webhooks
SET disabled_at = now(), disabled_reason = sqlc.arg(reason), last_error = sqlc.arg(last_error)
WHERE id = sqlc.arg(id);

-- name: ListJobChangesForWebhook :many
-- One page of the public feed (ListJobChanges, same order, same finished-transactions bound,
-- same privacy rule) plus company_slug, which the worker filters on. Kept separate so the
-- public feed's row stays exactly what GET /jobs/changes serves.
SELECT c.id, c.xid, c.kind, c.reason, c.at, j.public_slug, j.company_slug
FROM job_changes c
JOIN jobs j ON j.id = c.job_id
WHERE (c.xid, c.id) > (sqlc.arg(after_xid)::bigint, sqlc.arg(after_id)::bigint)
  AND c.xid < pg_snapshot_xmin(pg_current_snapshot())::text::bigint
  AND (NOT j.is_private OR c.kind = 'closed')
ORDER BY c.xid, c.id
LIMIT sqlc.arg(row_limit);
