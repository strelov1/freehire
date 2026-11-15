-- migrate: no-transaction
-- The synchronous fuzzy check at insert (jobdedup.CanonicalForFuzzy) reads one company's open
-- unclaimed (id, title) pairs for every INSERTED row. Without this the read walks
-- jobs_open_company_created_at_id_idx over every open row of the company; with it the read walks
-- only the candidate rows in id order and stops at its LIMIT. The predicate is the one
-- FuzzyDedupCandidateTitlesForCompany names (private postings are not in the catalogue), so the
-- batch pass's per-company read is served by it too. A PARTIAL index, so a row leaves it the
-- moment an exact pass claims it or it closes. title is deliberately NOT included: nothing caps
-- its length, and an over-long title would exceed the btree tuple limit and fail the insert.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_fuzzy_candidate_idx
    ON public.jobs (company_slug, id)
    WHERE closed_at IS NULL AND duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL AND NOT is_private;
