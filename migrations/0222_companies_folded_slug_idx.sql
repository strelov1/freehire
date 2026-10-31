-- migrate: no-transaction
--
-- An index on the FOLDED company slug, for the "Hiring again" filter (jobs search's
-- former_employers=true), which resolves the free-text company names of a person's
-- experience bank to catalogue companies on the request path.
--
-- The question it asks is "which companies carrying jobs fold to this key?" — every slug
-- whose replace(slug, '-', '') equals the name's normalize.CompanyKey. The answer has to be
-- ALL of them, not the one exact slug, because a key two different slugs fold to
-- (`dollar-tree` / `dollartree`, `vk` / `v-k`) is a name the catalogue itself has not
-- settled, and the filter refuses such a name rather than guessing. Without this index that
-- question is a sequential scan of ~800k companies per search.
--
-- An expression index rather than a stored column like jobs.company_slug_folded (0109). That
-- column exists because, on jobs, the planner had no usable estimate for
-- `replace(company_slug, '-', '') = ANY($param)` and drove each batch off a different, wrong
-- index. companies offers no competing selective index for this read, and the reader joins
-- the keys through unnest so each key is one index probe; a stored column would mean a table
-- rewrite or a value every companies write path must remember to maintain, for a table this
-- small.

-- CONCURRENTLY (hence the no-transaction marker): companies is upserted on every ingest
-- write, and a plain CREATE INDEX would hold those writes for the build.
CREATE INDEX CONCURRENTLY IF NOT EXISTS companies_folded_slug_idx
    ON public.companies (replace(slug, '-', ''));
