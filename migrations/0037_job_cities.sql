-- cities facet: canonical city names resolved deterministically from the location
-- string by internal/location (via internal/jobderive), mirroring countries/regions.
-- A source fact stored beside the enrichment JSONB, so the LLM worker never clobbers
-- it. Unlike the other geography facets, the SERVED cities facet is a hybrid: jobview
-- backfills from the LLM's enrichment.cities when this deterministic column is empty
-- (a deliberate coverage exception — cities are high-cardinality and the dict is a
-- beacon list), so this column holds the dictionary values only.
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS cities TEXT[] NOT NULL DEFAULT '{}';
