-- Serves the sitemap's freshest-jobs feed (ListJobSitemapFreshest:
-- WHERE closed_at IS NULL ORDER BY id DESC LIMIT n). Ordering by id DESC over the
-- open-jobs partial index reads the newest rows in physical heap order — a
-- sequential, cache-warm scan (~0.5s for 50k) — where a created_at-ordered index
-- scatters heap access (~17s) and a full-catalogue enumeration is heap-bound and
-- pollutes the buffer cache. Partial on the open predicate every list surface
-- shares (closed_at IS NULL), so it stays small.
--
-- Prod note: this runs on first initdb here, but on the live table apply it with
-- CREATE INDEX CONCURRENTLY (outside a transaction) so the build does not lock writes:
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_id_idx ON jobs (id) WHERE closed_at IS NULL;
CREATE INDEX IF NOT EXISTS jobs_open_id_idx
    ON jobs (id)
    WHERE closed_at IS NULL;
