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
