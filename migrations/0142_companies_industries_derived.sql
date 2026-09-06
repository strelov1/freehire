-- The domain-count-limited half of the derived-industry facet (issue #2088; see the
-- limit-derived-industry-domain-count change). #2082 stopped the industries facet's
-- derived arm from overruling a company's own curated `industries`. It did nothing
-- for a company with NO curated industry at all, where the job-derived `domains`
-- union — aggregated over every open job — is the only source consulted: a company
-- with many postings accumulates domains describing its hiring range rather than its
-- business.
--
-- industries_derived is the materialized, precedence-and-threshold-applied form of
-- that facet: RefreshCompanyFacets / cmd/recount-companies fills it with the
-- company's domains translated to industries ONLY when the company has no curated
-- industry AND carries at most two distinct domains; otherwise it is empty. Baking
-- both rules in at recompute time (rather than checking them per request) is what
-- lets both query backends (Postgres, Meilisearch) filter `industries` with a plain
-- OR against this column, with no runtime cardinality/emptiness check.
--
-- The GIN index this column needs (queried the same way as `industries`/`domains`:
-- array-contains equality against a requested value) is 0143, its own no-transaction
-- file — CONCURRENTLY cannot share a file with this statement.
--
-- Applied to a fresh volume by initdb after 0141; on an existing prod volume this
-- statement must be run manually BEFORE deploying code that reads the column, then
-- cmd/recount-companies backfills every company and a standalone reindex-companies
-- run (never stacked with `make reindex`) is required before the column is usable as
-- a Meilisearch filter.

ALTER TABLE public.companies
    ADD COLUMN industries_derived text[] DEFAULT '{}'::text[] NOT NULL;
