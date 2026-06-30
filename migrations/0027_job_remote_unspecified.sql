-- remote_unspecified facet (see openspec change add-remote-region-filters): a
-- deterministic boolean, derived at ingest by internal/jobderive, true when a job
-- is remote (work_mode='remote') but its geography resolved to no country and no
-- region — the "remote, region not specified" bucket. Like the other dictionary
-- facets it is a SOURCE fact stored beside (not inside) the `enrichment` JSONB, so
-- the LLM enrichment worker never clobbers it; jobview serves it dict-only and the
-- search index registers it as a filterable attribute. Defaults false so existing
-- rows are inert until re-derived (cmd/backfill-derive) and reindexed.
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS remote_unspecified BOOLEAN NOT NULL DEFAULT false;
