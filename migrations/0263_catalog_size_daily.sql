-- The catalogue's size, one row per UTC day: how many postings the public listings could
-- page through (open, not duplicate-suppressed, not private — catalogstats' predicate).
--
-- A measured row is written by cmd/rollup-stats from the exact snapshot it publishes; a
-- later run on the same day replaces it, so a day holds its last measurement. Days before
-- the table existed are filled once by cmd/backfill-catalog-history from today's jobs and
-- pruned_jobs and carry reconstructed = true: they apply today's duplicate verdicts to the
-- past, so they are an estimate and every reader must be able to say so. A measured row
-- always wins over a reconstructed one.
CREATE TABLE catalog_size_daily (
    day           date        PRIMARY KEY,
    open_jobs     bigint      NOT NULL,
    reconstructed boolean     NOT NULL,
    computed_at   timestamptz NOT NULL DEFAULT now()
);
