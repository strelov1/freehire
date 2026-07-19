-- name: EnsureBalance :exec
-- Seed a balance row for a brand-new user so the subsequent SELECT ... FOR UPDATE
-- always has a row to lock (this is what serializes concurrent first-ever debits).
-- For an existing user the row is left untouched; a stale period is reset later
-- under the lock. remaining is seeded with the monthly grant for a fresh row.
INSERT INTO credit_balances (user_id, period, remaining)
VALUES (sqlc.arg(user_id), sqlc.arg(period), sqlc.arg(remaining))
ON CONFLICT (user_id) DO NOTHING;

-- name: GetBalanceForUpdate :one
-- Lock the caller's balance row for the debit transaction. EnsureBalance guarantees
-- the row exists, so this never returns no-rows on the debit path; the lock serializes
-- concurrent debits for the same user so the balance can never be oversold.
SELECT period, remaining
FROM credit_balances
WHERE user_id = $1
FOR UPDATE;

-- name: InsertGrant :exec
-- Record the monthly grant for (user, period), idempotent via the partial unique index
-- on (user_id, period) WHERE kind = 'grant'. Safe to call on every debit: it inserts
-- once per period and does nothing thereafter. Purchase grants (kind='purchase') are
-- exempt from this index and can be added later without contention.
INSERT INTO credit_ledger (user_id, period, kind, feature, delta)
VALUES (sqlc.arg(user_id), sqlc.arg(period), 'grant', NULL, sqlc.arg(delta))
ON CONFLICT (user_id, period) WHERE kind = 'grant' DO NOTHING;

-- name: DebitExists :one
-- Whether the caller already spent points on this (feature, ref). True means the action
-- is a recompute/resume and must not be charged again (idempotency by ref).
SELECT EXISTS (
    SELECT 1 FROM credit_ledger
    WHERE user_id = sqlc.arg(user_id)
      AND kind = 'debit'
      AND feature = sqlc.arg(feature)::text
      AND ref = sqlc.arg(ref)::text
);

-- name: InsertDebit :exec
-- Append the debit for a metered action. delta is negative (the action cost). The partial
-- unique index on (user_id, feature, ref) WHERE kind='debit' guards against a double charge
-- for the same ref even under a race.
INSERT INTO credit_ledger (user_id, period, kind, feature, delta, ref)
VALUES (sqlc.arg(user_id), sqlc.arg(period), 'debit', sqlc.arg(feature)::text, sqlc.arg(delta), sqlc.arg(ref)::text);

-- name: RewardExists :one
-- Whether the caller already received a reward for this ref (e.g. an accepted contribution).
-- True means the reward was already granted and must not be granted again (idempotency).
SELECT EXISTS (
    SELECT 1 FROM credit_ledger
    WHERE user_id = sqlc.arg(user_id)
      AND kind = 'reward'
      AND ref = sqlc.arg(ref)::text
);

-- name: InsertReward :exec
-- Append a reward: points earned (e.g. for an accepted board contribution), delta positive,
-- feature NULL. Rewards bank above the monthly grant and survive the period reset.
INSERT INTO credit_ledger (user_id, period, kind, feature, delta, ref)
VALUES (sqlc.arg(user_id), sqlc.arg(period), 'reward', NULL, sqlc.arg(delta), sqlc.arg(ref)::text);

-- name: UpdateBalance :exec
-- Persist the post-transaction balance: the current period and remaining points. Called at
-- the end of the debit transaction to write back any lazy reset and/or decrement. The row is
-- guaranteed to exist (EnsureBalance ran first).
UPDATE credit_balances
SET period = sqlc.arg(period), remaining = sqlc.arg(remaining), updated_at = now()
WHERE user_id = sqlc.arg(user_id);

-- name: GetBalance :one
-- Read-only balance for display (no lock, no LLM). Returns no rows for a user who has never
-- had credit activity; the caller treats that as a full monthly grant remaining.
SELECT period, remaining
FROM credit_balances
WHERE user_id = $1;

-- name: ListCreditLedger :many
-- The caller's credit-ledger entries, newest first, for the transaction-history page. Bounded
-- by a caller-supplied limit and served by the (user_id, created_at DESC) index. The handler
-- resolves each debit's ref to a human label (the job/CV it named).
SELECT kind, feature, delta, ref, created_at
FROM credit_ledger
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListJobLabelsByIDs :many
-- Resolve job ids to display labels for the credit-history page (match debits). Missing ids
-- simply do not come back; the handler falls back to a generic label for a deleted job.
SELECT id, title, public_slug
FROM jobs
WHERE id = ANY(sqlc.arg(ids)::bigint[]);

-- name: ListTailoredCVLabelsByIDs :many
-- Resolve tailored-CV ids to their target job's display labels for the credit-history page
-- (tailor debits). Only tailored CVs (job_id set) whose job still exists resolve; the handler
-- falls back to a generic label otherwise.
SELECT c.id, j.title AS job_title, j.public_slug AS job_slug
FROM cvs c
JOIN jobs j ON j.id = c.job_id
WHERE c.id = ANY(sqlc.arg(ids)::bigint[]);
