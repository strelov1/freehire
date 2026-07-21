-- View-count aggregation from nginx access logs. An offline worker (cmd/rollup-views)
-- reads completed, rotated access-log day-files and counts job views off the read path,
-- so serving a job never writes a counter. Two tables back it:
--
--   job_daily_views    -- per-job unique daily views (dedup by hashed IP+UA within a day,
--                         the day taken from each line's timestamp). Applied additively;
--                         the running sum of `uniques` is materialized into jobs.view_count.
--   processed_view_logs -- the cursor: which rotated files are fully aggregated, keyed by a
--                          signature of the file's decompressed content (FNV-64 hash). Content
--                          is stable across BOTH logrotate's numeric-suffix rename (.1 -> .2)
--                          AND its later gzip (a new inode, same bytes), so a file is
--                          identified once and daily runs and repeated backfills are idempotent.
--
-- jobs.view_count already exists (0001_init); its meaning widens from "distinct signed-in
-- viewers" to "distinct daily visitors across all traffic" — the worker maintains it now,
-- and POST /jobs/:slug/view no longer bumps it.
--
-- Applied to a fresh volume by initdb after 0036; on an existing prod volume run these
-- statements manually BEFORE deploying the worker.

CREATE TABLE public.job_daily_views (
    day     date   NOT NULL,
    job_id  bigint NOT NULL,
    uniques integer NOT NULL,
    PRIMARY KEY (day, job_id)
);

CREATE TABLE public.processed_view_logs (
    signature    bigint      PRIMARY KEY,
    filename     text        NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now()
);
