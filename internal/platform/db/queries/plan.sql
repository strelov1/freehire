-- name: GetProUntil :one
-- The whole of a user's plan. A timestamp in the future means pro; NULL or a past one
-- means free. Read on the request path of every metered action, which is why it is a
-- column read and never a call to a billing provider: a provider that is slow must not
-- be able to slow down a user's next question.
SELECT pro_until
FROM users
WHERE id = $1;

-- name: SetProUntilStripe :exec
-- How far the WEB subscription reaches. Written only by the Stripe sync, from Stripe's
-- current view of the customer.
--
-- It writes a source, not the plan. users.pro_until is derived by the schema as the furthest
-- of three sources and refuses assignment outright (428C9) — see migration 0135 — so
-- clearing this column says "Stripe confers nothing", never "this account is not Pro". The
-- account may hold a store subscription or a manual grant, and before 0135 this write would
-- have revoked either without a trace.
UPDATE users
SET pro_until_stripe = sqlc.narg(until)
WHERE id = sqlc.arg(id);

-- name: SetProUntilRevenueCat :exec
-- How far the APP STORE or GOOGLE PLAY subscription reaches. Written only by the RevenueCat
-- sync, and only over its own source column, for the same reason the Stripe setter is
-- confined to its own: neither provider may answer for a plan it did not sell.
UPDATE users
SET pro_until_revenuecat = sqlc.narg(until)
WHERE id = sqlc.arg(id);

-- name: SetProUntilGranted :exec
-- Pro GIVEN rather than sold: support's manual grant today, awarded days once add-invites
-- lands. No provider sync touches it, which is the whole reason it is separate — before
-- migration 0135 a hand-set value lived in the column the Stripe sync overwrites, and the
-- next webhook silently undid it.
UPDATE users
SET pro_until_granted = sqlc.narg(until)
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

-- name: ReleaseConsumption :execrows
-- Void a consumption taken as a RESERVATION for work that then produced nothing.
--
-- It RESTAMPS the row rather than deleting it or adding a compensating entry, and the
-- shape of the index is what makes that the right move. `usage_ledger_consume_ref_uniq` is
-- scoped to kind='consume', so:
--
--   * appending a compensating row would leave the original standing, the ref permanently
--     spent, and the user's retry charged nothing — free work, forever;
--   * deleting the row frees the ref but erases the fact that a reservation was ever
--     taken, which is exactly the kind of hole an append-only ledger exists to prevent;
--   * restamping frees the ref AND keeps the row, so the history reads "charged, then
--     returned" and the day's counter — which sums only kind='consume' — is correct.
--
-- Returns the number of rows restamped, so the caller gives the allowance back exactly
-- when it really took one: a double release, or a release of something already voided,
-- matches nothing and returns 0. That is what lets every failure path call this without
-- first working out whether it owes one.
UPDATE usage_ledger
SET kind = 'release'
WHERE user_id = sqlc.arg(user_id)
  AND kind = 'consume'
  AND feature = sqlc.arg(feature)::text
  AND ref = sqlc.arg(ref)::text;

-- name: ListConsumptionRefsByPrefix :many
-- The live consumption references a user holds beginning with a prefix. This is what makes
-- the tailoring turn ceiling need no column of its own: a session's charges are written as
-- '<session_id>#1', '#2', and so on, so the SLOT NUMBERS say how many ceilings the session
-- holds and the ceiling in force is the highest of them times the per-session turn allowance.
--
-- It returns the references rather than counting them, because a count and a slot number
-- are not the same answer. A session that predates this metering holds no row at all and is
-- given slot 1 implicitly, so its first extension buys slot 2 — under a count that extension
-- would have read back as one ceiling and bought the session nothing. The same gap opens
-- whenever a row is released: the count drops while the slots already sold do not.
--
-- The suffix is parsed by the caller, in the file that writes it, so one place owns the
-- format. A session holds a handful of rows, so there is nothing to aggregate away.
--
-- The prefix is passed already terminated by the caller (the session id plus '#'), so a
-- session id that is a prefix of another cannot borrow its charges.
SELECT ref
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

-- name: GetProUntilSources :one
-- The plan and where it came from, in one read.
--
-- The derived column and its three sources together, because the surface needs both: the
-- instant to show, and which origin equals it. Two queries would be two round trips for one
-- row, and — worse — could disagree if a sync landed between them.
SELECT pro_until, pro_until_stripe, pro_until_revenuecat, pro_until_granted
FROM users
WHERE id = $1;
