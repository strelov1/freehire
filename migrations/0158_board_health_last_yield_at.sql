-- Records WHEN a board last actually yielded a posting, as distinct from when its crawl last
-- SUCCEEDED (last_success_at).
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
-- THE BACKFILL IS LOAD-BEARING, NOT COSMETIC. Seeding the existing fleet with last_success_at
-- is what makes an empty column safe to ship. A reader must treat "no yield recorded" as
-- evidence of an empty feed for the mechanism to work at all — and on the day this lands,
-- every one of the 155k rows would carry exactly that, having never had the chance to record
-- one. Any consumer measuring an age window would then qualify the ENTIRE catalogue at once.
-- The seed starts every board's clock at its last known-good crawl instead, so no board can be
-- judged empty until it has been observably empty for a full window AFTER this deploys. A row
-- that has never succeeded keeps NULL, which is the honest reading: it is unreachable, not
-- empty, and that is already the chronic net's job.
ALTER TABLE board_health ADD COLUMN last_yield_at timestamptz;

COMMENT ON COLUMN board_health.last_yield_at IS
    'When this board last yielded at least one posting. Distinct from last_success_at, which an '
    'empty-but-reachable feed refreshes on every run. NULL means no yield has been observed '
    'since the column was added; consumers must not read that as an empty feed.';

UPDATE board_health SET last_yield_at = last_success_at WHERE last_success_at IS NOT NULL;
