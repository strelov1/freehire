-- migrate: no-transaction
--
-- Records WHEN a board last actually yielded a posting, as distinct from when its crawl last
-- SUCCEEDED (last_success_at).
--
-- OUTSIDE A TRANSACTION because the ALTER and the backfill must not share one. board_health is
-- written continuously by the whole crawl fleet — every board of every provider stamps it on
-- every run — and the ALTER takes ACCESS EXCLUSIVE, which conflicts with all of that. Inside a
-- transaction that lock would be held until COMMIT, i.e. across a 155k-row UPDATE, turning an
-- instant DDL into a fleet-wide stall and a likely 55P03. Split, the ALTER holds its lock for
-- the microsecond it needs (Postgres 11+ does not rewrite the table for a nullable column) and
-- the UPDATE that follows takes only ROW EXCLUSIVE, which no crawl blocks on.
--
-- The cost of splitting is that a crawl can land between the two statements; the backfill's own
-- `last_yield_at IS NULL` guard is what keeps it from overwriting the stamp that crawl earned.
--
-- The two came apart in production on 2026-09-10. A WhatJobs market whose publisher account had
-- been emptied answered every request with HTTP 200 and an empty result set: board_health
-- recorded success on every run, consecutive_failures stayed 0, and the unit stayed green —
-- while the provider's stored postings sat open for a month pointing at a feed that no longer
-- carried them. Neither existing safety net could reach them. The ordinary per-run sweep is
-- gated on shouldSweep (cmd/ingest), which requires the run to have ingested at least one job,
-- because "a run that ingested nothing proves only that the crawl failed" — true of a broken
-- crawl, false of a feed that answered honestly with zero. And the chronic-board net
-- (cmd/close-chronic-boards) measures last_success_at, which an empty-but-reachable feed
-- refreshes every single run, so it never qualifies either. A permanently EMPTY board was the
-- exact twin of the permanently UNREACHABLE board issue #2017 already solved, and nothing
-- measured it.
--
-- Written only by a crawl that REACHED at least one posting (pipeline.boardReachedPostings:
-- ingested, rejected or already-covered — every outcome that proves the listing named a
-- posting). Skipped is excluded for the reason boardReachedPostings states in full: it means
-- the posting was listed and then failed to PERSIST, so counting it would let a board whose
-- every save is failing prove itself on the strength of its own persistence failures.
--
-- TWO SEPARATE GUARDS KEEP AN EMPTY COLUMN FROM CLOSING THE CATALOGUE, and it is worth being
-- precise about which does what, because they are easy to conflate:
--
--   1. NULL never qualifies (ListEmptyFeedBoards' first condition). This is what makes the
--      column safe on day one. Of the 155,678 rows in production, 145,963 are reachable and
--      inside any plausible window — every one of them would qualify as "empty" the moment a
--      consumer read a missing stamp as evidence. It does not: NULL means "no yield has been
--      OBSERVED", which is a statement about our measurement, not about the feed.
--   2. The backfill below is what makes the mechanism WORK. Guard 1 alone would leave a board
--      that is already empty at deploy time stuck on NULL forever — it can never earn a stamp,
--      since earning one requires yielding — so the very boards this exists for would be the
--      ones it could never reach. Seeding from last_success_at starts each board's clock at its
--      last known-good crawl, so an already-empty board ages out of the window on its own, and
--      no board can be judged empty until it has been observably empty for a full window AFTER
--      this deploys.
--
-- A row that has never succeeded keeps NULL, which is the honest reading: it is unreachable,
-- not empty, and that is already the chronic net's job (cmd/close-chronic-boards' first pass).
-- IF NOT EXISTS because there is no transaction to roll this back: if the backfill below fails
-- part way, the re-run has to get past this line rather than stop on a column it already added.
-- The UPDATE is rerunnable for the same reason, through its own IS NULL guard.
ALTER TABLE board_health ADD COLUMN IF NOT EXISTS last_yield_at timestamptz;

COMMENT ON COLUMN board_health.last_yield_at IS
    'When this board last yielded at least one posting. Distinct from last_success_at, which an '
    'empty-but-reachable feed refreshes on every run. NULL means no yield has been observed '
    'since the column was added; consumers must not read that as an empty feed.';

-- `last_yield_at IS NULL` guards the seed rather than merely filtering it: without a transaction
-- a crawl can land between the ALTER above and this statement and stamp a genuine yield, and an
-- unguarded seed would overwrite that real measurement with an older timestamp.
UPDATE board_health
SET last_yield_at = last_success_at
WHERE last_yield_at IS NULL AND last_success_at IS NOT NULL;
