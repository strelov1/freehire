-- Indexes for the newest-first list surfaces and the enrichment claim, whose ORDER BYs no
-- existing index served. jobs_posted_at_id_idx (0001) is keyed on posted_at, but ListJobs /
-- ListJobsByCompany sort by created_at and ClaimEnrichmentBatch by COALESCE(posted_at,
-- created_at). Without these, the public /api/v1/jobs list and CountJobs full-scan open jobs
-- and top-N sort on every request, and a version-bump enrichment backfill re-sorts the whole
-- claimable set on every wave. All three are partial on the open-jobs predicate every list
-- surface shares (closed_at IS NULL), so they stay small and the filter is free.
--
-- Prod note: these run on first initdb here, but on the live table apply them with
-- CREATE INDEX CONCURRENTLY (outside a transaction) so the build does not lock writes:
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_created_at_id_idx          ON jobs (created_at DESC, id DESC)                        WHERE closed_at IS NULL;
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_company_created_at_id_idx  ON jobs (company_slug, created_at DESC, id DESC)          WHERE closed_at IS NULL;
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_enrich_freshness_idx       ON jobs ((COALESCE(posted_at, created_at)) DESC, id DESC) WHERE closed_at IS NULL;

-- Serves ListJobs (WHERE closed_at IS NULL ORDER BY created_at DESC, id DESC) and CountJobs
-- (a count of the partial index's entries).
CREATE INDEX IF NOT EXISTS jobs_open_created_at_id_idx
    ON jobs (created_at DESC, id DESC)
    WHERE closed_at IS NULL;

-- Serves ListJobsByCompany (WHERE company_slug = $1 AND closed_at IS NULL ORDER BY
-- created_at DESC, id DESC): company_slug leads so the equality seeks, then the index yields
-- the newest-first order directly.
CREATE INDEX IF NOT EXISTS jobs_open_company_created_at_id_idx
    ON jobs (company_slug, created_at DESC, id DESC)
    WHERE closed_at IS NULL;

-- Serves ClaimEnrichmentBatch's ORDER BY COALESCE(posted_at, created_at) DESC, id DESC over
-- open jobs, so a claim wave is an ordered index scan + LIMIT instead of a full sort of the
-- whole claimable set. The expression mirrors the query's freshness key exactly.
CREATE INDEX IF NOT EXISTS jobs_open_enrich_freshness_idx
    ON jobs ((COALESCE(posted_at, created_at)) DESC, id DESC)
    WHERE closed_at IS NULL;
