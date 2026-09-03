-- SUPERSEDED ON ANY DATABASE THAT RECORDED THIS FILE — see 0127.
--
-- This file was rewritten after it had already been recorded as applied on prod, which
-- is the one thing the migration rule forbids. The rewrite is honest about what went
-- wrong and useless where it matters: a recorded version row means this file is never
-- read again, so everything below about dropping a carcass fixes only databases that
-- never had one. 0127 is what actually repaired prod, and it carries the full account.
-- Read this file for the index's shape and the CONCURRENTLY reasoning; read 0127 for
-- what ran.
--
-- The global discussions feed (GET /api/v1/threads/recent, /discussions): every open
-- thread across every subject, newest first, keyset-paged on (created_at, id).
--
-- threads_subject_open_created_idx cannot serve it. That index leads with
-- (subject_type, subject_ref), so an unfiltered scan ordered by created_at has no
-- usable prefix and falls back to a full scan plus a sort. This index is the same
-- shape without the subject columns.
--
-- Partial on status = 'open' for the same reason as the subject index: a closed thread
-- is hidden from every default listing, so it never belongs in the hot index.
--
-- NOT concurrently, and the DROP above it is why. The first version of this file used
-- CREATE INDEX CONCURRENTLY — the shape this repo settled on for an index a running
-- prod builds — and it failed the release on 2026-09-03 with SQLSTATE 55P03, a lock
-- timeout. CONCURRENTLY does not merely take a weaker lock: before building, it WAITS
-- for every transaction holding an older snapshot to finish, and migrate's 5s
-- lock_timeout applies to that wait. Two similar-jobs backfill transactions were six
-- minutes into their run, so the wait could never complete inside five seconds. The
-- release aborted correctly and left the live colour untouched, but a failed
-- CONCURRENTLY leaves the index behind with indisvalid = false, and prod is carrying
-- exactly that carcass now.
--
-- `threads` holds three rows. A plain CREATE INDEX takes ACCESS EXCLUSIVE on it for as
-- long as indexing three rows takes, and nothing else contends for that table — which
-- makes CONCURRENTLY here pure cost: it buys no availability worth having and imports
-- a dependency on how long an unrelated backfill's transaction happens to be. The
-- concurrent shape is right for jobs (7.4M rows, read on every request); it is the
-- wrong reflex on a table this size, and reaching for it out of habit is what cost a
-- release. Revisit if this table ever grows.
--
-- No `migrate: no-transaction` marker, so the two statements are one transaction: the
-- invalid index cannot be dropped without its replacement landing.

-- Drops the invalid index the failed CONCURRENTLY left on prod. Unconditional rather
-- than guarded, because an IF NOT EXISTS on the CREATE alone would find that carcass,
-- skip, and leave an index nothing can use. A no-op on a fresh volume.
-- squawk-ignore require-concurrent-index-deletion -- three rows, and a concurrent drop cannot run inside this file's transaction
DROP INDEX IF EXISTS threads_open_created_idx;

-- squawk-ignore require-concurrent-index-creation -- three rows; CONCURRENTLY here is what failed the 2026-09-03 release, see above
CREATE INDEX IF NOT EXISTS threads_open_created_idx
    ON public.threads (created_at DESC, id DESC)
    WHERE status = 'open';
