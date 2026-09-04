-- name: ListSchedulableProviders :many
-- Every provider the scheduler may run, with its override if it has one.
--
-- The roster is boards, and the LEFT JOIN is what makes ingest_schedule a set of
-- OVERRIDES rather than the roster: a provider with a live board and no schedule row comes
-- back with NULLs, which the caller resolves to documented defaults. An INNER JOIN here
-- would silently unschedule every unconfigured provider, which is the exact failure this
-- table was built to remove.
SELECT b.provider,
       s.shards,
       s.cadence_sec,
       s.timeout_sec,
       s.enabled,
       s.disabled_reason,
       s.notes,
       s.managed
FROM (SELECT DISTINCT provider FROM boards WHERE status IN ('pending', 'active')) b
LEFT JOIN ingest_schedule s ON s.provider = b.provider
ORDER BY b.provider;

-- name: EnsureRunStateShards :exec
-- Materialise the (provider, 1..shards) rows a provider needs. ON CONFLICT DO NOTHING is
-- load-bearing: an existing shard keeps its next_due_at, which is the fleet's stagger, and
-- resetting it would bunch a provider's whole cycle onto one minute.
--
-- A new shard is due immediately. That is deliberate — a shard that has never run has no
-- schedule to respect, and the concurrency cap is what keeps a fresh 24-way provider from
-- taking the whole fleet at once.
INSERT INTO ingest_run_state (provider, shard, next_due_at)
SELECT sqlc.arg(provider), g, now()
FROM generate_series(1, sqlc.arg(shards)::int) AS g
ON CONFLICT (provider, shard) DO NOTHING;

-- name: DeleteSurplusRunStateShards :exec
-- Drop the shards left over from a higher shard count.
--
-- A CLAIMED row is left alone. Deleting one would erase the scheduler's only record that a
-- crawl is still executing, so the fleet would under-count itself and launch past its cap —
-- and the surviving run would finish with nothing to report to. The row goes on the next
-- tick after it is reaped, which costs one minute and cannot lose a slot.
DELETE FROM ingest_run_state
WHERE provider = sqlc.arg(provider)
  AND shard > sqlc.arg(shards)::int
  AND claimed_at IS NULL;

-- name: DeleteRunStateForUnlistedProviders :exec
-- Forget the providers that are no longer eligible — every board retired, or the adapter
-- gone. This is the sweep gen-ingest-timers.sh promised in its header and never had: under
-- it, a provider's timer survived forever and kept crawling nothing (careerspage ran empty
-- from 18 July).
--
-- A CLAIMED row survives, for the same reason as the surplus-shard delete above: it is the
-- only record that a crawl is still running, and losing it makes the fleet under-count
-- itself. A provider disabled mid-crawl keeps its row for one more tick, until the reap.
DELETE FROM ingest_run_state
WHERE provider <> ALL (sqlc.arg(providers)::text[])
  AND claimed_at IS NULL;

-- name: ClaimDueRuns :many
-- Take up to max_runs due runs, exactly once each.
--
-- The CTE resolves each candidate's cadence and timeout through the same LEFT JOIN and
-- defaults as the listing above, so a claim can never use different numbers from the
-- report. FOR UPDATE ... SKIP LOCKED is what makes two overlapping scheduler ticks safe:
-- the second skips the rows the first holds rather than blocking on them or double-claiming.
-- `OF rs` names only the run-state table, since FOR UPDATE may not be applied to the
-- nullable side of an outer join.
--
-- A row is claimable when it is due and unclaimed, or when its claim has outlived that
-- provider's own timeout plus the grace window — a scheduler killed between claiming and
-- launching, and a run systemd killed at its timeout, both recover through that second arm
-- with no operator.
--
-- next_due_at advances to now() + cadence, not to next_due_at + cadence. Advancing at
-- claim stops a 40-minute crawl from halving its own frequency; advancing from now() caps
-- catch-up at ONE run, so a six-hour outage does not owe six.
WITH candidate AS (
    SELECT rs.provider,
           rs.shard,
           -- The shard COUNT is how many run-state rows the provider has, not
           -- ingest_schedule.shards. Reconcile creates exactly one row per shard, so the
           -- rows ARE the count; reading it from the override table instead would be a
           -- second source for one fact, and the two disagree for any provider whose rows
           -- exist while its override row does not. A subquery rather than a window
           -- function, because FOR UPDATE may not be combined with one.
           (SELECT count(*) FROM ingest_run_state peer WHERE peer.provider = rs.provider)::int AS shards,
           COALESCE(s.cadence_sec, sqlc.arg(default_cadence_sec)::int) AS cadence_sec,
           COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int) AS timeout_sec
    FROM ingest_run_state rs
    LEFT JOIN ingest_schedule s ON s.provider = rs.provider
    WHERE ((rs.claimed_at IS NULL AND rs.next_due_at <= now())
        OR (rs.claimed_at IS NOT NULL
            AND rs.claimed_at < now() - make_interval(
                    secs => COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int)
                            + sqlc.arg(grace_sec)::int)))
    -- ROLLOUT GATE, removed with the column in task 8.5 of
    -- openspec/changes/ingest-scheduler-in-db. Run state is tracked for every enabled
    -- provider so the stagger and the shadow preview exist from day one; what `managed`
    -- decides is whether the SCHEDULER may launch it, or whether its static timer still
    -- owns it. COALESCE to false: while the column exists, a provider nobody has handed
    -- over is still the static timer's.
    AND COALESCE(s.managed, false)
    ORDER BY rs.next_due_at
    LIMIT sqlc.arg(max_runs)
    FOR UPDATE OF rs SKIP LOCKED
)
UPDATE ingest_run_state rs
SET claimed_at      = now(),
    last_started_at = now(),
    next_due_at     = now() + make_interval(secs => c.cadence_sec)
FROM candidate c
WHERE rs.provider = c.provider AND rs.shard = c.shard
RETURNING rs.provider, rs.shard, c.shards, c.timeout_sec;

-- name: RecordRunFinish :exec
-- Store how a run ended and release its claim, so the row is claimable again at its next
-- due time. Clearing claimed_at here is what keeps the reclaim window for genuinely stuck
-- runs rather than for every run that took a while.
UPDATE ingest_run_state
SET claimed_at       = NULL,
    last_finished_at = now(),
    last_exit_code   = sqlc.arg(exit_code)::int,
    last_error       = NULLIF(sqlc.arg(last_error)::text, '')
WHERE provider = sqlc.arg(provider) AND shard = sqlc.arg(shard)::int;

-- name: ListInFlightRuns :many
-- Every claimed run, with what the scheduler needs to ask the service manager about it.
--
-- Rows, not a count. A transient unit finishes and tells nobody, so claimed_at is set at
-- claim and cleared by nothing until the scheduler reaps: a plain count would include every
-- run that ever succeeded, and the fleet's concurrency cap would fill permanently after
-- Cap launches with every check still green.
--
-- This is what replaces ingest-slot.sh's flock semaphore. 279 independent timers could not
-- see each other, so the ceiling had to live in a wrapper script; one scheduler can count —
-- but only if it also notices when a run has ended.
SELECT rs.provider,
       rs.shard,
       (SELECT count(*) FROM ingest_run_state peer WHERE peer.provider = rs.provider)::int AS shards,
       COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int) AS timeout_sec
FROM ingest_run_state rs
LEFT JOIN ingest_schedule s ON s.provider = rs.provider
WHERE rs.claimed_at IS NOT NULL
ORDER BY rs.claimed_at;

-- name: PreviewDueRuns :many
-- What ClaimDueRuns WOULD take, without taking it. Shadow mode's read: the first
-- deployment lands underneath a fleet still driven by the static timers, so a tick that
-- advanced a due time would desynchronise state the real timers know nothing about.
--
-- The predicate is copied from ClaimDueRuns rather than shared, because sqlc has no way to
-- share one. A divergence between the two would make the shadow run a measurement of
-- something other than what apply mode does, so they are asserted equivalent by an
-- integration test rather than by inspection.
SELECT rs.provider,
       rs.shard,
       (SELECT count(*) FROM ingest_run_state peer WHERE peer.provider = rs.provider)::int AS shards,
       COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int) AS timeout_sec
FROM ingest_run_state rs
LEFT JOIN ingest_schedule s ON s.provider = rs.provider
WHERE ((rs.claimed_at IS NULL AND rs.next_due_at <= now())
    OR (rs.claimed_at IS NOT NULL
        AND rs.claimed_at < now() - make_interval(
                secs => COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int)
                        + sqlc.arg(grace_sec)::int)))
    -- ROLLOUT GATE, removed with the column in task 8.5 of
    -- openspec/changes/ingest-scheduler-in-db. Run state is tracked for every enabled
    -- provider so the stagger and the shadow preview exist from day one; what `managed`
    -- decides is whether the SCHEDULER may launch it, or whether its static timer still
    -- owns it. COALESCE to false: while the column exists, a provider nobody has handed
    -- over is still the static timer's.
    AND COALESCE(s.managed, false)
ORDER BY rs.next_due_at
LIMIT sqlc.arg(max_runs);

-- name: ReportIngestSchedule :many
-- The whole schedule as an operator reads it: every eligible provider, its override if it
-- has one, and what its runs have actually been doing. Aggregated per provider rather than
-- per shard, because the question this answers is "is anything not running?" and 24
-- paylocity rows would bury the answer.
--
-- shards_in_state is counted from run state rather than read from the override, for the
-- same reason ClaimDueRuns counts it: the rows ARE the shard count, and a report that read
-- the intended number instead would show a healthy 24 while 12 rows existed.
SELECT b.provider,
       s.shards,
       s.cadence_sec,
       s.timeout_sec,
       s.enabled,
       s.disabled_reason,
       s.notes,
       s.managed,
       COALESCE(rs.shards_in_state, 0)::int AS shards_in_state,
       COALESCE(rs.in_flight, 0)::int       AS in_flight,
       rs.next_due_at,
       rs.last_finished_at
FROM (SELECT DISTINCT provider FROM boards WHERE status IN ('pending', 'active')) b
LEFT JOIN ingest_schedule s ON s.provider = b.provider
LEFT JOIN (
    SELECT provider,
           count(*)                                        AS shards_in_state,
           count(*) FILTER (WHERE claimed_at IS NOT NULL)  AS in_flight,
           min(next_due_at)::timestamptz                   AS next_due_at,
           max(last_finished_at)::timestamptz              AS last_finished_at
    FROM ingest_run_state
    GROUP BY provider
) rs ON rs.provider = b.provider
ORDER BY b.provider;

-- name: UpsertIngestSchedule :exec
-- Write one provider's override. Every argument is optional: a NULL means "leave this
-- alone" on an existing row and "use the documented default" on a new one, so a curator
-- changing only the shard count does not silently reset the cadence someone measured.
--
-- The CHECK on the table still decides whether the result is legal — disabling without a
-- reason is refused here exactly as it is in psql, which is the point of putting the rule
-- in the schema.
INSERT INTO ingest_schedule (provider, shards, cadence_sec, timeout_sec,
                             enabled, disabled_reason, notes, managed)
VALUES (sqlc.arg(provider),
        COALESCE(sqlc.narg(shards)::int, 1),
        COALESCE(sqlc.narg(cadence_sec)::int, 3600),
        COALESCE(sqlc.narg(timeout_sec)::int, 3000),
        COALESCE(sqlc.narg(enabled)::boolean, true),
        sqlc.narg(disabled_reason)::text,
        sqlc.narg(notes)::text,
        COALESCE(sqlc.narg(managed)::boolean, false))
ON CONFLICT (provider) DO UPDATE SET
    shards          = COALESCE(sqlc.narg(shards)::int, ingest_schedule.shards),
    cadence_sec     = COALESCE(sqlc.narg(cadence_sec)::int, ingest_schedule.cadence_sec),
    timeout_sec     = COALESCE(sqlc.narg(timeout_sec)::int, ingest_schedule.timeout_sec),
    enabled         = COALESCE(sqlc.narg(enabled)::boolean, ingest_schedule.enabled),
    disabled_reason = COALESCE(sqlc.narg(disabled_reason)::text, ingest_schedule.disabled_reason),
    notes           = COALESCE(sqlc.narg(notes)::text, ingest_schedule.notes),
    managed         = COALESCE(sqlc.narg(managed)::boolean, ingest_schedule.managed),
    updated_at      = now();
