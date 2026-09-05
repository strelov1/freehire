-- What decides that a provider's crawl is due. The board catalog moved into Postgres in
-- 0123 and sources/ was retired in #2406, but the SCHEDULE stayed in deploy/bin/
-- gen-ingest-timers.sh: a script that materialises ~279 static systemd units and that
-- nothing on the host invokes. Between its manual runs the schedule is a photograph of a
-- catalog that has since moved, and every divergence is silent — a provider with no
-- boards and a provider with no timer both exit 0. See
-- openspec/changes/ingest-scheduler-in-db/.
--
-- This table is a set of OVERRIDES, never the roster. The roster is `boards`: the distinct
-- providers holding a pending/active row. A provider with NO row here is scheduled on the
-- defaults below. That is the whole point — if this table were the roster, then "nobody
-- added a row" and "we decided not to crawl this" would be the same state, which is
-- exactly what hid two dead providers in production (habr_career crawled nothing for a
-- day because its unit was named after a FILE; careerspage ran empty from 18 July).
--
-- cadence_sec / timeout_sec are seconds rather than `interval`, because sqlc maps interval
-- to pgtype.Interval and that type would then travel through every caller for no gain;
-- cmd/schedule-board accepts "3h" and stores 10800.
CREATE TABLE ingest_schedule (
    provider        text PRIMARY KEY,
    -- 1 = crawl the provider whole. >1 partitions it across --shard=i/n runs, which is how
    -- a board list too large for one timeout is covered (paylocity: 24).
    -- Bounded ABOVE as well as below. The reconcile materialises one row per shard through
    -- generate_series, so an unbounded count turns a typo into a statement that outlives
    -- the scheduler's own start timeout — and every following tick repeats it first, which
    -- stops the whole fleet. 64 is comfortably past the largest real value (paylocity's 24)
    -- and far short of anything that could not finish.
    -- squawk-ignore prefer-bigint-over-int -- a shard count is tens; the largest is 24
    shards          int NOT NULL DEFAULT 1 CHECK (shards > 0 AND shards <= 64),
    -- squawk-ignore prefer-bigint-over-int -- seconds in a period; int32 spans 68 years
    cadence_sec     int NOT NULL DEFAULT 3600 CHECK (cadence_sec > 0),
    -- TimeoutStartSec for the launched run. 3000 = 2400s of crawl budget plus the
    -- historical 600s slot wait; a per-posting-detail provider needs 4500.
    -- squawk-ignore prefer-bigint-over-int -- a run budget in seconds; the largest is 4500
    timeout_sec     int NOT NULL DEFAULT 3000 CHECK (timeout_sec > 0),
    enabled         boolean NOT NULL DEFAULT true,
    disabled_reason text,
    -- What was MEASURED to justify the numbers above. A calibrated fleet decays into
    -- folklore the moment a number outlives the measurement that produced it.
    notes           text,
    -- ROLLOUT-ONLY, dropped by task 8.5 of this change. While the static timers still run,
    -- the scheduler launches only providers flipped to managed, so the two cannot both
    -- drive one provider. It defaults false, which INVERTS the absence rule above; leaving
    -- it in place after cutover would restore the very failure this table removes.
    managed         boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    -- Not crawling a provider must be a decision someone wrote down. psql is a writer too,
    -- so the rule is the table's, not the one Go caller's. btrim, not <> '': a reason of
    -- three spaces is not a reason.
    CONSTRAINT ingest_schedule_disabled_needs_reason
        CHECK (enabled OR btrim(coalesce(disabled_reason, '')) <> '')
);

COMMENT ON TABLE ingest_schedule IS
    'Per-provider ingest scheduling OVERRIDES. The roster is boards; a provider absent '
    'from this table is scheduled on the column defaults. enabled=false requires a '
    'disabled_reason. managed is rollout-only and is dropped once every provider is cut '
    'over — see openspec/changes/ingest-scheduler-in-db/tasks.md task 8.5.';
