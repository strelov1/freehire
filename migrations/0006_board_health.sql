-- Per-board ingest health: a runtime-state sidecar keyed by the crawl identity
-- (provider, board). It is NOT a catalog — the set of boards to crawl and their
-- cadence stay in the YAML board files (git owns those). This table only remembers
-- each board's last outcome so a repeatedly-failing board can be cooled down (skipped)
-- instead of hammered every run, and so an operator can see which boards are failing.
-- Boardless/aggregator entries use board = '' (their single identity).
CREATE TABLE board_health (
    provider             text NOT NULL,
    board                text NOT NULL,
    consecutive_failures integer NOT NULL DEFAULT 0,
    cooldown_until       timestamptz,
    last_error           text,
    last_error_at        timestamptz,
    last_success_at      timestamptz,
    last_ingested_count  integer,
    last_run_at          timestamptz,
    PRIMARY KEY (provider, board)
);
