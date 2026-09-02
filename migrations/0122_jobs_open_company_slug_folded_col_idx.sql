-- migrate: no-transaction
--
-- Partial index on the STORED jobs.company_slug_folded column.
--
-- This index has existed on prod since migration 0109 landed, but only because 0109's comment
-- told an operator to build it CONCURRENTLY by hand — the file never created it. Any volume
-- built from the migrations (initdb, a testcontainers run, a rebuilt prod) therefore has the
-- column and no index on it, and every reader of the column falls back to a sequential scan
-- over ~7.4M rows. Recording the index in a file is what makes the two agree.
--
-- 0076's expression index is NOT a substitute: it is keyed on replace(company_slug, '-', ''),
-- which the planner cannot match to a `company_slug_folded = $1` predicate. The column exists
-- precisely because the expression form is unusable once the value arrives as a parameter
-- (0109 carries the measurements: 271s per batch against 491ms over the plain column).
--
-- The readers: cmd/reindex's aggregator-suppression pass, and — new, and why this is being
-- recorded now rather than left as prod-only drift — CompaniesWithFreshNonAggregatorCoverage,
-- which cmd/ingest runs once per board run. A sequential scan there is a per-crawl cost on
-- every aggregator board, not a once-a-night one.
--
-- CONCURRENTLY (hence the no-transaction marker) rather than a plain CREATE INDEX with a
-- comment telling an operator to do it by hand, which is the shape 0076 used and the shape
-- that lost this index in the first place. IF NOT EXISTS makes it a no-op on prod, where the
-- hand-built index already carries this exact name and definition; on an empty volume
-- CONCURRENTLY costs a second pass over no rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS jobs_open_company_slug_folded_col_idx
    ON public.jobs (company_slug_folded)
    WHERE closed_at IS NULL AND company_slug <> '';
