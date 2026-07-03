-- Authoritative company-info attributes, loaded by the one-time cmd/backfill-company-info
-- worker from an external dataset. These are per-company facts (headcount, founding year,
-- HQ, industry, organization type) that our own jobs can't supply, kept independent of the
-- job-derived facet columns (company_types/company_sizes/countries/domains/regions): the
-- periodic RefreshCompanyFacets never touches these, and the backfill never touches those.
-- Unknown source values stay NULL (or absent from the JSONB) so "unknown" is distinguishable
-- from a real value.
ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS industries        TEXT[]      NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS year_founded      INT,
    ADD COLUMN IF NOT EXISTS employee_count    INT,
    ADD COLUMN IF NOT EXISTS hq_country        TEXT,
    ADD COLUMN IF NOT EXISTS organization_type TEXT,
    ADD COLUMN IF NOT EXISTS tagline           TEXT,
    -- Lower-coverage extras (homepage, funding, stock listing, parent, subsidiaries,
    -- activities); homepage goes here, not the job-derived domains[] that RefreshCompanyFacets
    -- owns, so it's available without clobbering.
    ADD COLUMN IF NOT EXISTS company_info      JSONB       NOT NULL DEFAULT '{}',
    -- A reference row is a company imported by the backfill that no job references (yet).
    -- The orphan cleanup preserves these; a later job for the same slug adopts the row.
    ADD COLUMN IF NOT EXISTS is_reference      BOOLEAN     NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS company_info_at   TIMESTAMPTZ;

-- Backs the future industries facet filter (array overlap &&), mirroring the other
-- companies facet GIN indexes.
CREATE INDEX IF NOT EXISTS companies_industries_idx ON companies USING GIN (industries);
