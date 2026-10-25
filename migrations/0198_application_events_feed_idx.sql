-- migrate: no-transaction
--
-- Serves the interview-buddies feed (openspec/changes/interview-buddies, D5): a keyset
-- read over the newest applied / stage_set events that have not been retracted. The
-- partial predicate keeps the index to the two kinds the feed reads. CONCURRENTLY needs
-- its own file because Postgres forbids it inside a transaction block, and application_events
-- is written to by every mail sync, so a plain build would block those writes.
--
-- On an existing prod volume build it by hand, detached from the SSH session (systemd-run
-- or nohup): a CONCURRENTLY build dies with its session and leaves an INVALID index
-- behind, and IF NOT EXISTS matches by name only, so a rerun would silently no-op on it.
-- Check pg_index.indisvalid before trusting a rerun; if invalid, DROP INDEX CONCURRENTLY it first.
CREATE INDEX CONCURRENTLY IF NOT EXISTS application_events_feed_idx
    ON application_events (occurred_at DESC, id DESC)
    WHERE kind IN ('applied', 'stage_set') AND retracted_at IS NULL;
