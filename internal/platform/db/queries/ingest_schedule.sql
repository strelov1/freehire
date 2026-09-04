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
DELETE FROM ingest_run_state
WHERE provider = sqlc.arg(provider) AND shard > sqlc.arg(shards)::int;

-- name: DeleteRunStateForUnlistedProviders :exec
-- Forget the providers that are no longer eligible — every board retired, or the adapter
-- gone. This is the sweep gen-ingest-timers.sh promised in its header and never had: under
-- it, a provider's timer survived forever and kept crawling nothing (careerspage ran empty
-- from 18 July).
DELETE FROM ingest_run_state
WHERE provider <> ALL (sqlc.arg(providers)::text[]);

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
    WHERE (rs.claimed_at IS NULL AND rs.next_due_at <= now())
       OR (rs.claimed_at IS NOT NULL
           AND rs.claimed_at < now() - make_interval(
                   secs => COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int)
                           + sqlc.arg(grace_sec)::int))
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

-- name: CountInFlightRuns :one
-- How many runs the scheduler believes are executing. This is what replaces
-- ingest-slot.sh's flock semaphore: 279 independent timers could not see each other, so
-- the ceiling had to live in a wrapper script; one scheduler can simply count.
SELECT count(*) FROM ingest_run_state WHERE claimed_at IS NOT NULL;

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
WHERE (rs.claimed_at IS NULL AND rs.next_due_at <= now())
   OR (rs.claimed_at IS NOT NULL
       AND rs.claimed_at < now() - make_interval(
               secs => COALESCE(s.timeout_sec, sqlc.arg(default_timeout_sec)::int)
                       + sqlc.arg(grace_sec)::int))
ORDER BY rs.next_due_at
LIMIT sqlc.arg(max_runs);
