-- migrate: no-transaction
--
-- Partial index behind the company page's Closed tab (ListClosedJobsByCompany).
--
-- jobs_company_slug_idx (company_slug, posted_at DESC NULLS LAST, id DESC) is not partial, so
-- it reaches closed rows too — but closed_at, duplicate_of and is_private are heap filters on
-- it, and page 1 walks every OPEN posting of the company before it finds a closed one.
-- Measured on prod, cold: dominos skipped 14.5k open rows in 5.4s, autozone 16.4k in 14.8s,
-- and lowes holds 23.6k. The cost scales with what the company is hiring for now, which is
-- exactly the set this tab does not show.
--
-- Partial on the tab's own exclusions, so the index holds only the rows the query can return
-- and the walk starts at the first one. Keyed (company_slug, closed_at DESC, id DESC) because
-- the tab lists most recently closed first — the order a reader of a closed list wants, and
-- the one this index serves without a sort. The query's ORDER BY must stay exactly this.
--
-- CONCURRENTLY (hence the no-transaction marker): jobs is the hot table and a plain CREATE
-- INDEX would block every crawl write for the length of the build. IF NOT EXISTS makes a
-- retried run after a partial failure a no-op once the index exists.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_closed_company_closed_at_idx
    ON public.jobs (company_slug, closed_at DESC, id DESC)
    WHERE closed_at IS NOT NULL AND duplicate_of IS NULL AND NOT is_private;
