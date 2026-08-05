-- Partial expression index backing SuppressAggregatorDuplicatesForCompany's cross-source
-- company match (internal/db/queries/jobs.sql). company_slug is normalize.Slug(name) — plain
-- transliteration and hyphenation, no legal-suffix stripping (internal/normalize/slug.go) —
-- so two sources spelling the same employer with a different word break ("Cfoinsights" vs
-- "CFO Insights") produce different slugs ("cfoinsights" vs "cfo-insights"). The suppression
-- pass now compares replace(company_slug, '-', ''); without this index that predicate
-- seq-scans the whole jobs table once per company in the reindex loop.
--
-- Applied to a fresh volume by initdb after 0075; on an existing prod volume build it
-- CONCURRENTLY out of band (a plain CREATE INDEX would lock the live jobs table):
--   CREATE INDEX CONCURRENTLY jobs_open_company_slug_folded_idx
--     ON public.jobs (replace(company_slug, '-', ''))
--     WHERE closed_at IS NULL AND company_slug <> '';
CREATE INDEX IF NOT EXISTS jobs_open_company_slug_folded_idx
    ON public.jobs (replace(company_slug, '-', ''))
    WHERE closed_at IS NULL AND company_slug <> '';
