-- migrate: no-transaction
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
-- CONCURRENTLY (hence the no-transaction marker) is the convention this repo settled on
-- for an index a running prod will build — it costs a second pass and no exclusive lock.
-- The table is tiny today, so the cost is nil either way and the habit is what matters.
CREATE INDEX CONCURRENTLY IF NOT EXISTS threads_open_created_idx
    ON public.threads (created_at DESC, id DESC)
    WHERE status = 'open';
