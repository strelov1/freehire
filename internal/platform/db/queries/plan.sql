-- name: GetProUntil :one
-- The whole of a user's plan. A timestamp in the future means pro; NULL or a past one
-- means free. Read on the request path of every metered action, which is why it is a
-- column read and never a call to a billing provider: a provider that is slow must not
-- be able to slow down a user's next question.
SELECT pro_until
FROM users
WHERE id = $1;

-- name: SetProUntil :exec
-- Move a user's plan expiry. The only writer today is a hand-run statement; the billing
-- webhook and its reconciler become the writers in the change that adds them, and they
-- write this and nothing else.
UPDATE users
SET pro_until = sqlc.narg(pro_until)
WHERE id = sqlc.arg(id);

-- name: EnsureUsageDay :exec
-- Seed today's counter for (user, feature) so the SELECT ... FOR UPDATE below always has
-- a row to lock. That lock is what serialises two simultaneous first-ever consumptions,
-- so an allowance can never be oversold by a race. An existing row is left untouched.
INSERT INTO usage_daily (user_id, feature, day, used)
VALUES (sqlc.arg(user_id), sqlc.arg(feature)::text, sqlc.arg(day), 0)
ON CONFLICT (user_id, feature, day) DO NOTHING;

-- name: GetUsageDayForUpdate :one
-- Lock today's counter for the consumption transaction. EnsureUsageDay guarantees the row
-- exists, so this never returns no-rows on that path.
SELECT used
FROM usage_daily
WHERE user_id = sqlc.arg(user_id)
  AND feature = sqlc.arg(feature)::text
  AND day = sqlc.arg(day)
FOR UPDATE;

-- name: GetUsageDay :one
-- Today's counter for one feature, read without a lock, for a surface that wants to say
-- where the caller stands before offering an action. No rows means the feature has not
-- been touched today, which the caller reports as untouched rather than as absent.
SELECT used
FROM usage_daily
WHERE user_id = sqlc.arg(user_id)
  AND feature = sqlc.arg(feature)::text
  AND day = sqlc.arg(day);

-- name: SetUsageDay :exec
-- Persist the post-transaction counter. The row is guaranteed to exist (EnsureUsageDay
-- ran first). Yesterday's rows are simply left behind rather than reset: a day that has
-- rolled over is answered by a different key, so nothing has to expire anything and no
-- scheduled job exists to forget to run.
UPDATE usage_daily
SET used = sqlc.arg(used), updated_at = now()
WHERE user_id = sqlc.arg(user_id)
  AND feature = sqlc.arg(feature)::text
  AND day = sqlc.arg(day);

-- name: ConsumptionExists :one
-- Whether this (feature, ref) was already charged. True means the caller is retrying,
-- reconnecting, or recomputing work already paid for, and must not be charged again.
SELECT EXISTS (
    SELECT 1 FROM usage_ledger
    WHERE user_id = sqlc.arg(user_id)
      AND kind = 'consume'
      AND feature = sqlc.arg(feature)::text
      AND ref = sqlc.arg(ref)::text
);

-- name: InsertConsumption :exec
-- Append the consumption for a metered action; delta is positive and counts units taken.
-- The partial unique index on (user_id, feature, ref) WHERE kind='consume' guards against
-- a double charge for the same ref even under a race.
INSERT INTO usage_ledger (user_id, feature, day, kind, delta, ref)
VALUES (sqlc.arg(user_id), sqlc.arg(feature)::text, sqlc.arg(day), 'consume', sqlc.arg(delta), sqlc.arg(ref)::text);

-- name: GetConsumptionDay :one
-- Which day a consumption was recorded against, read WITHOUT a lock.
--
-- A release must decrement the counter of the day the charge landed on, not of the day
-- the release happens — otherwise a reservation taken at 23:59 and released at 00:01
-- gives back an allowance the user never spent today, and the day it really spent stays
-- spent. Reading it first is also what keeps the lock order the same in both directions:
-- every path takes usage_daily before usage_ledger, so a consumption and a release for
-- the same user cannot deadlock each other.
--
-- No rows means nothing was charged under this reference, and the release is a no-op.
SELECT day
FROM usage_ledger
WHERE user_id = sqlc.arg(user_id)
  AND kind = 'consume'
  AND feature = sqlc.arg(feature)::text
  AND ref = sqlc.arg(ref)::text;

-- name: DeleteConsumption :execrows
-- Void a consumption taken as a RESERVATION for work that then produced nothing.
--
-- It deletes rather than appending a compensating row, and the unique index forces that:
-- at most one consumption may exist per (user, feature, ref), so a compensating entry
-- would leave the ref permanently spent and the user's retry would find it already
-- charged and never re-reserve. Deleting frees the ref.
--
-- Returns the number of rows removed, so the caller gives the allowance back exactly when
-- it really took one — a double release, or a release of something already voided,
-- removes nothing and returns 0. That is what lets every failure path call this without
-- first working out whether it owes one.
DELETE FROM usage_ledger
WHERE user_id = sqlc.arg(user_id)
  AND kind = 'consume'
  AND feature = sqlc.arg(feature)::text
  AND ref = sqlc.arg(ref)::text;

-- name: CountConsumptionsByRefPrefix :one
-- How many consumptions a user holds for refs beginning with a prefix. This is what makes
-- the tailoring turn ceiling need no column of its own: a session's charges are written
-- as '<session_id>#1', '#2', and so on, so counting them gives the number of ceilings
-- bought, and the ceiling in force is that count times the per-session turn allowance.
--
-- The prefix is passed already terminated by the caller (the session id plus '#'), so a
-- session id that is a prefix of another cannot borrow its charges.
SELECT count(*)
FROM usage_ledger
WHERE user_id = sqlc.arg(user_id)
  AND kind = 'consume'
  AND feature = sqlc.arg(feature)::text
  AND ref LIKE sqlc.arg(ref_prefix)::text || '%';

-- name: ListUsageForDay :many
-- Every feature's consumption for one user on one day, for the usage surface. A feature
-- the user has not touched today simply does not come back, and the caller reports it as
-- untouched rather than as absent.
SELECT feature, used
FROM usage_daily
WHERE user_id = sqlc.arg(user_id)
  AND day = sqlc.arg(day);

-- name: ListUsageLedger :many
-- The caller's ledger entries, newest first, bounded by a caller-supplied limit and served
-- by the (user_id, created_at DESC) index.
SELECT feature, day, kind, delta, ref, created_at
FROM usage_ledger
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: CountAssistantUserTurns :one
-- How many turns a session has run, counted as the candidate's own messages.
--
-- This is what both metered questions about a session are asked against: whether a
-- tailoring session has reached its turn ceiling, and which turn a charge belongs to. It
-- counts user rows only — an answer, a tool call and its result are all one turn's work,
-- and counting them would make a turn's price depend on how the model chose to do it.
SELECT count(*)
FROM assistant_messages
WHERE session_id = $1
  AND role = 'user';

-- name: ListTailoredCVLabelsBySessions :many
-- Resolve tailoring-session ids to their vacancy's display labels for the usage history.
--
-- A tailoring charge names the SESSION, not the CV — that is what the turn ceiling is
-- counted from — so the label has to come back through the binding on the CV row. Only a
-- tailored copy whose vacancy still exists resolves; anything else simply does not come
-- back, and the caller falls back to a generic label rather than inventing one.
SELECT c.agent_session_id, j.title AS job_title, j.public_slug AS job_slug
FROM cvs c
JOIN jobs j ON j.id = c.job_id
WHERE c.user_id = sqlc.arg(user_id)
  AND c.agent_session_id = ANY(sqlc.arg(session_ids)::text[]);

-- name: DeleteUsageForUser :exec
-- Erase one user's consumption ledger. Account deletion calls this alongside the counter
-- delete; the foreign keys cascade, but deletion states what it erases explicitly rather
-- than relying on a constraint to mean it.
DELETE FROM usage_ledger WHERE user_id = $1;

-- name: DeleteUsageDailyForUser :exec
-- Erase one user's daily counters. See DeleteUsageForUser.
DELETE FROM usage_daily WHERE user_id = $1;
