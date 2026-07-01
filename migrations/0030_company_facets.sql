-- Denormalized facet arrays per company, derived from the company's open jobs so
-- the /companies catalog can be filtered by collection/region/country/industry/
-- type/size without joining jobs on the hot read path. Like companies.job_count
-- (0025), these are maintained by the periodic recompute (cmd/recount-companies),
-- not a write-path trigger, so they are eventually consistent with jobs.
--
-- regions/countries come from the top-level jobs.regions/jobs.countries columns
-- (dense from ingest); domains/company_types/company_sizes come from the
-- jobs.enrichment JSONB (sparse until a job is LLM-enriched). Each is the distinct
-- union across the company's open jobs. Default '{}' so a company with no open
-- jobs (or no enriched jobs) reads as empty between recomputes.
ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS regions        TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS countries      TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS domains        TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS company_types  TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS company_sizes  TEXT[] NOT NULL DEFAULT '{}';

-- GIN indexes back the array-overlap (&&) filters the list endpoint uses; without
-- them a facet filter would sequentially scan companies.
CREATE INDEX IF NOT EXISTS companies_regions_idx       ON companies USING GIN (regions);
CREATE INDEX IF NOT EXISTS companies_countries_idx     ON companies USING GIN (countries);
CREATE INDEX IF NOT EXISTS companies_domains_idx       ON companies USING GIN (domains);
CREATE INDEX IF NOT EXISTS companies_company_types_idx ON companies USING GIN (company_types);
CREATE INDEX IF NOT EXISTS companies_company_sizes_idx ON companies USING GIN (company_sizes);
