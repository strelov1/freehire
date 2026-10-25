-- Quiet-board backoff (openspec change quiet-board-backoff): a board whose listing — the SET of
-- postings it names — has not changed since its previous complete successful crawl is crawled
-- progressively less often (pipeline.QuietFor: 4h doubling to 72h). Every crawl used to re-read
-- every board; on 2026-10-25 close-chronic-boards counted 7,229 boards whose crawls all succeed
-- and yield nothing, re-read every cycle by a fleet whose capacity is the host's bottleneck.
--
-- listing_fingerprint: sha256 of the board's sorted, distinct posting external ids from its last
--   COMPLETE successful crawl. NULL until a complete crawl records one.
-- quiet_streak: consecutive complete crawls that found the fingerprint unchanged.
-- quiet_until: the board is skipped until this instant; NULL = not quiet.
--
-- quiet_until is a separate column from cooldown_until on purpose: ClearProviderCooldowns (the
-- recovery probe after a provider-wide outage) nulls cooldown_until for every cooled board of a
-- provider at once, and sharing the column would erase every quiet pause on most runs.
--
-- One ALTER, three columns: a nullable column and a constant default are both catalogue-only on
-- Postgres 11+, so ACCESS EXCLUSIVE is held only for the instant the catalogue changes, against
-- a table the whole crawl fleet writes continuously.
ALTER TABLE board_health
    ADD COLUMN IF NOT EXISTS listing_fingerprint text,
    -- squawk-ignore prefer-bigint-over-int -- counts crawls (a few a day at most), like consecutive_failures beside it
    ADD COLUMN IF NOT EXISTS quiet_streak integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS quiet_until timestamptz;
