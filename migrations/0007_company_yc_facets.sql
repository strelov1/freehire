-- Curated YC directory facets on companies (see the yc-company-enrichment change).
-- yc_batch (e.g. 'Winter 2012') and yc_status (e.g. 'Active') are loaded from the
-- yc-oss directory by cmd/import-yc — curated company facts, NOT job-derived. Like
-- the other curated facets they are NOT maintained by RefreshCompanyFacets /
-- cmd/recount-companies, which never references them, so an imported value survives
-- every recompute. Modelled as text[] (single-element in practice) so they filter
-- through the same array-overlap facet machinery as regions/collections.
--
-- Applied to a fresh volume by initdb after 0006; on an existing prod volume these
-- statements must be run manually BEFORE deploying code that reads the columns.
-- Additive with a non-NULL default — every existing company reads as empty until
-- the importer annotates it.

ALTER TABLE public.companies
    ADD COLUMN yc_batch text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN yc_status text[] DEFAULT '{}'::text[] NOT NULL;
