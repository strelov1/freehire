-- migrate: no-transaction
-- The synchronous fuzzy check at insert (jobdedup.CanonicalForFuzzy) reads one company's open
-- unclaimed (id, title) pairs for every INSERTED row. Without this the read walks
-- jobs_open_company_created_at_id_idx and visits the heap for every open row of the company
-- (the title is not in any index); with it the same read is an index-only scan in id order that
-- stops at its LIMIT. The predicate is the one FuzzyDedupCandidateTitlesForCompany names (private postings are not in the catalogue, so they are out of both roles), so the
-- batch pass's per-company read is served by it too. A PARTIAL index, so a row leaves it the
-- moment an exact pass claims it or it closes. Not in the key: title (INCLUDE only), so a title
-- edit does not re-sort the tree.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_fuzzy_candidate_idx
    ON public.jobs (company_slug, id) INCLUDE (title)
    WHERE closed_at IS NULL AND duplicate_of_aggregator IS NULL AND duplicate_of_role IS NULL AND NOT is_private;
