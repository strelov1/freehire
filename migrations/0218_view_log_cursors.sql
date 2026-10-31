-- How far into a view-log file cmd/rollup-views has got, so the worker can commit a file
-- in bounded chunks instead of one transaction.
--
-- Each applied row also adds to jobs.view_count, and that UPDATE holds the jobs row's lock
-- until its transaction commits. Applying a whole file in one transaction held ~1M jobs
-- rows locked for 45 minutes (the nginx file) and ~1.4M for another 71 (the agent-view
-- file) on 2026-10-31, and every ingest write touching one of them queued behind it — the
-- crawl fleet stalled nightly. Committing a few hundred rows at a time frees each row in
-- seconds, but leaves a failed run part-applied, and the counts are ADDITIVE, so a re-run
-- that started over would count the applied part twice. This table is what prevents it:
-- the worker applies a file's rows in ascending (job_id, day) order and, in the SAME
-- transaction as each chunk, moves the file's cursor to the chunk's last key. A re-run
-- resumes after the cursor. The move is a compare-and-swap against the cursor the run
-- read, so two runs over one file cannot both apply a chunk.
--
-- A file is finished when processed_view_logs holds its signature, as before. Its cursor
-- row is kept rather than deleted: a run that read "not processed" just before another
-- finished would otherwise find no cursor and start over.
CREATE TABLE IF NOT EXISTS public.view_log_cursors (
    signature  bigint      PRIMARY KEY,
    filename   text        NOT NULL,
    job_id     bigint      NOT NULL,
    day        date        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
