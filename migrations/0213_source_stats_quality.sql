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
-- NULLABLE, and NULL means NOT YET MEASURED — the browsable_jobs rule, for the same reason.
-- The rows already in the table were measured before these columns existed; a default of 0
-- would have queue-metrics publish "nothing missing" for every source, under their old
-- measured_at, until the next rebuild. The rebuild always writes all five, and
-- SourceQualityMetrics skips a row that has none.
ALTER TABLE source_stats
    ADD COLUMN missing_work_mode_jobs bigint,
    ADD COLUMN missing_geo_jobs       bigint,
    ADD COLUMN missing_company_jobs   bigint,
    ADD COLUMN unseen_7d_jobs         bigint,
    ADD COLUMN posted_over_60d_jobs   bigint;
