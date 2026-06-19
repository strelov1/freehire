-- Curated collections (see specs/add-collections-pages): editorial, company-level
-- themes (e.g. yc, bigtech) that are facts ABOUT the company — who funded it, how
-- prominent it is — not derivable from a job's text or its ATS source (Airbnb/Dropbox
-- are YC but hire through their own Greenhouse, not Work at a Startup).
--
-- companies.collections is the membership source of truth, populated by
-- cmd/import-collections from external datasets. jobs.collections is a denormalized
-- copy of the owning company's set, mirroring how company_slug is denormalized onto
-- jobs "so a company's jobs are a single-table filter (no join)". It feeds the
-- Meilisearch `collections` facet; the import worker propagates it and a reindex
-- surfaces it (same operational caveat as the other dictionary facets).
--
-- Both default to '{}' (NOT NULL) so an untagged company/job carries an empty set,
-- and UpsertJob — which never names these columns — leaves an existing job's
-- collections untouched on re-ingest.
ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS collections TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS collections TEXT[] NOT NULL DEFAULT '{}';
