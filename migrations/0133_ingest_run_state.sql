-- Where each scheduled run currently stands. Machine-owned, and split from
-- ingest_schedule (0132) for the same reason boards is split from board_health: a
-- curator's decision must not be clobbered by a run, and a run's outcome must not be lost
-- when a curator edits a cadence. The claim UPDATE here fires every minute; schedule edits
-- must not contend with it.
--
-- Keyed (provider, shard) because sharding is what makes a provider too large for one
-- timeout crawlable at all: shard 3 of paylocity becomes due independently of shard 4.
-- An unsharded provider is simply shard 1 of 1, so there is one shape, not two.
--
-- next_due_at is advanced at CLAIM, to now() + cadence, not at finish and not from its own
-- previous value. Advancing at claim stops a 40-minute crawl from silently halving its own
-- frequency; advancing from now() caps catch-up at ONE run, so a six-hour scheduler outage
-- does not owe six runs that would stampede the fleet the moment it returns.
CREATE TABLE ingest_run_state (
    provider         text NOT NULL,
    -- squawk-ignore prefer-bigint-over-int -- a shard ordinal within its provider; max 24
    shard            int NOT NULL CHECK (shard > 0),
    next_due_at      timestamptz NOT NULL,
    -- Set when a tick takes the row, cleared when the run's outcome is recorded. A claim
    -- older than its provider's timeout plus a grace window is treated as dead and may be
    -- taken again — that is how a scheduler killed between claiming and launching, and a
    -- run systemd killed at its timeout, both recover without an operator.
    claimed_at       timestamptz,
    last_started_at  timestamptz,
    last_finished_at timestamptz,
    -- squawk-ignore prefer-bigint-over-int -- a process exit status is 0-255
    last_exit_code   int,
    last_error       text,

    PRIMARY KEY (provider, shard)
);

-- The claim query's exact predicate: the earliest unclaimed rows that are due. Partial, so
-- the index holds only claimable rows rather than the whole fleet.
CREATE INDEX ingest_run_state_due_idx
    ON ingest_run_state (next_due_at)
    WHERE claimed_at IS NULL;

-- The reclaim query walks live claims, which are few — a full scan of that slice is
-- cheaper than a second index maintained on every claim and every finish.
CREATE INDEX ingest_run_state_claimed_idx
    ON ingest_run_state (claimed_at)
    WHERE claimed_at IS NOT NULL;

COMMENT ON TABLE ingest_run_state IS
    'Per (provider, shard) scheduling state: when the run is next due, whether a tick has '
    'claimed it, and how the last run ended. Machine-owned; curator settings live in '
    'ingest_schedule.';
