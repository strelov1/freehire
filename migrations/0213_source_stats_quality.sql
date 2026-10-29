-- Per-source quality counts beside open_jobs: how many of a source's open postings lack
-- what a visitor relies on (a work mode, a place, an employer) or are likely no longer
-- live (unseen by their crawl for a week, posted two months ago).
--
-- Measured in the same grouped scan cmd/rollup-stats already runs for open_jobs, so they
-- cost no extra pass over `jobs`; published by cmd/queue-metrics as
-- freehire_source_quality_gap. Raw counts, never shares: which share of which source
-- deserves an alert belongs to the alert rule, since several sources are legitimately
-- geo-less or never re-listed.
--
-- NOT NULL DEFAULT 0 is a metadata-only change, and the table holds one row per source.
-- The default only covers the rows already there until the next rebuild rewrites them.
ALTER TABLE source_stats
    ADD COLUMN missing_work_mode_jobs bigint NOT NULL DEFAULT 0,
    ADD COLUMN missing_geo_jobs       bigint NOT NULL DEFAULT 0,
    ADD COLUMN missing_company_jobs   bigint NOT NULL DEFAULT 0,
    ADD COLUMN unseen_7d_jobs         bigint NOT NULL DEFAULT 0,
    ADD COLUMN posted_over_60d_jobs   bigint NOT NULL DEFAULT 0;
