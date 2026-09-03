-- Repairs threads_open_created_idx on any database that recorded 0126 while the index
-- was invalid. On every other database this is a cheap rebuild of a three-row index.
--
-- The sequence that produced the state, because it is not obvious and it will recur:
--
--   1. 0126 shipped as CREATE INDEX CONCURRENTLY IF NOT EXISTS. On prod it hit
--      migrate's 5s lock_timeout — CONCURRENTLY waits for older snapshots before it
--      builds, and two similar-jobs backfill transactions were six minutes in. The
--      release aborted before flipping, correctly, and the migration was NOT recorded.
--      But a failed CONCURRENTLY leaves the index behind with indisvalid = false.
--   2. The host polls GitHub every ~10 minutes and deploys on its own. That autodeploy
--      re-ran 0126 — whose IF NOT EXISTS found the invalid index, skipped it without
--      error, and RECORDED the migration as applied.
--   3. 0126 was then edited to drop the carcass and build plainly. That edit is
--      correct, and it can never reach any database that completed step 2: the version
--      row is already there, so the file is never read again.
--
-- The lesson is the repo's existing rule, sharpened: "applied" is decided by a
-- schema_migrations row, and with a host that deploys itself every ten minutes, a
-- migration that failed a minute ago may be applied by the time the fix is written.
-- Editing an unapplied migration races the autodeploy. Adding a file does not.
--
-- Postgres will not use an invalid index, so until this runs the feed's listing falls
-- back to a scan and a sort. Harmless at three rows, and a lie in pg_indexes either way.
--
-- Plain, not CONCURRENTLY, for the reason 0126 now records: three rows, nothing else
-- contends for this table, and the concurrent form is what failed. No no-transaction
-- marker, so the drop and the create are atomic.

-- squawk-ignore require-concurrent-index-deletion -- three rows, and a concurrent drop cannot run inside this file's transaction
DROP INDEX IF EXISTS threads_open_created_idx;

-- squawk-ignore require-concurrent-index-creation -- three rows; CONCURRENTLY is what failed the 2026-09-03 release, see above
CREATE INDEX IF NOT EXISTS threads_open_created_idx
    ON public.threads (created_at DESC, id DESC)
    WHERE status = 'open';
