-- Trigram indexes backing the /companies name search (ListCompanies/CountCompanies).
-- The search predicate is `name ILIKE '%q%' OR slug ILIKE '%q%'` — a leading-wildcard
-- match, which is non-sargable: no btree can serve it, so the planner falls back to
-- reading every hiring row (job_count > 0, ~227k of them) out of the 2.3 GB heap and
-- applying the ILIKE per row. Measured cold that is ~14 s per keystroke (EXPLAIN showed
-- a parallel bitmap heap scan, ~546 MB read, work_mem-lossy bitmap), and CountCompanies
-- runs the same predicate again for the page total.
--
-- pg_trgm's gin_trgm_ops indexes each string's trigrams, so `ILIKE '%q%'` becomes a GIN
-- lookup that returns only the matching rows (hundreds) instead of scanning all 227k.
-- Two single-column indexes let the planner BitmapOr the name/slug arms of the OR.
--
-- Applied to a fresh volume by initdb after 0032. On the live prod volume the extension
-- and both indexes were built out of band — a plain CREATE INDEX would lock the live
-- companies table (workers upsert into it), so each index went in CONCURRENTLY, which
-- cannot run inside a transaction:
--   CREATE EXTENSION IF NOT EXISTS pg_trgm;
--   CREATE INDEX CONCURRENTLY companies_name_trgm_idx ON public.companies USING gin (name gin_trgm_ops);
--   CREATE INDEX CONCURRENTLY companies_slug_trgm_idx ON public.companies USING gin (slug gin_trgm_ops);
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS companies_name_trgm_idx
    ON public.companies USING gin (name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS companies_slug_trgm_idx
    ON public.companies USING gin (slug gin_trgm_ops);
