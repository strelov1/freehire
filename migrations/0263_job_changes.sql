-- The append-only record of a posting opening, closing and reopening, behind the public
-- change feed (GET /api/v1/jobs/changes).
--
-- Why a table and not a read of jobs.closed_at: a reopen CLEARS closed_at (the ingest upsert,
-- a moderator), so the row alone cannot say a posting was closed and came back; a mirror that
-- delisted it would never relist it. A trigger on jobs writes every transition, so each of the
-- closers (ingest sweeps, board-gone, liveness, deadline, moderators, close-chronic-boards)
-- and raw SQL are covered without touching them.
--
-- What is logged is a flip of "listed": open AND canonical (duplicate_of IS NULL) AND public
-- (NOT is_private), the rule the site and the search index apply. Dedup workers set the
-- duplicate_of_* owner columns AFTER the insert (a BEFORE trigger derives duplicate_of from
-- them), so a posting can stop being listed, or start, with closed_at untouched. A listed
-- posting that leaves is `closed` (reason: the close reason, else 'duplicate' or 'private');
-- one that returns is `reopened`. Judging visibility only when the feed is read would lose both.
--
-- `xid` is the writing transaction's id (xid8 as bigint). The feed pages by (xid, id) and
-- serves only rows below the oldest still-running transaction, so a long bulk close that
-- commits late can never land behind a cursor a client already holds. `id` alone cannot give
-- that: it is taken in the trigger, long before the commit. `at` is the event time, for
-- display and for resolving `since`.
--
-- There is deliberately no foreign key to jobs: nothing deletes jobs, and an FK would need an
-- index on job_id only to serve deletes. A row whose job is gone simply drops out of the feed.
-- No backfill: the log begins at this migration, and the feed reports where it begins.
CREATE TABLE job_changes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id bigint NOT NULL,
    kind text NOT NULL CHECK (kind IN ('opened', 'closed', 'reopened')),
    -- jobs.closed_reason at a close, or 'duplicate' / 'private'; '' for opened and reopened.
    reason text NOT NULL DEFAULT '',
    at timestamptz NOT NULL DEFAULT clock_timestamp(),
    xid bigint NOT NULL DEFAULT (pg_current_xact_id()::text::bigint)
);

-- The feed's keyset walk.
CREATE INDEX job_changes_xid_id_idx ON job_changes (xid, id);
-- Resolving `since` to a starting point, and the feed's earliest timestamp.
CREATE INDEX job_changes_at_idx ON job_changes (at);

CREATE FUNCTION public.jobs_log_change() RETURNS trigger
    LANGUAGE plpgsql AS $$
DECLARE
    was_listed boolean := false;
    is_listed boolean := NEW.closed_at IS NULL AND NEW.duplicate_of IS NULL AND NOT NEW.is_private;
BEGIN
    IF TG_OP = 'UPDATE' THEN
        was_listed := OLD.closed_at IS NULL AND OLD.duplicate_of IS NULL AND NOT OLD.is_private;
    END IF;
    IF is_listed = was_listed THEN
        RETURN NULL;
    END IF;
    IF is_listed THEN
        INSERT INTO job_changes (job_id, kind)
        VALUES (NEW.id, CASE WHEN TG_OP = 'INSERT' THEN 'opened' ELSE 'reopened' END);
    ELSE
        INSERT INTO job_changes (job_id, kind, reason)
        VALUES (NEW.id, 'closed', CASE
            WHEN NEW.closed_at IS NOT NULL THEN NEW.closed_reason
            WHEN NEW.duplicate_of IS NOT NULL THEN 'duplicate'
            ELSE 'private' END);
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER jobs_log_change_ins
    AFTER INSERT ON public.jobs
    FOR EACH ROW
    EXECUTE FUNCTION public.jobs_log_change();

-- duplicate_of is assigned by a BEFORE trigger from the three owner columns, and an
-- UPDATE OF list sees only the statement's own SET targets, so the owners are named too.
-- The WHEN keeps a no-op (a touch of an already-closed row, a duplicate moving owner) out.
CREATE TRIGGER jobs_log_change_upd
    AFTER UPDATE OF closed_at, is_private, duplicate_of,
        duplicate_of_aggregator, duplicate_of_role, duplicate_of_fuzzy ON public.jobs
    FOR EACH ROW
    WHEN ((OLD.closed_at IS NULL AND OLD.duplicate_of IS NULL AND NOT OLD.is_private)
        <> (NEW.closed_at IS NULL AND NEW.duplicate_of IS NULL AND NOT NEW.is_private))
    EXECUTE FUNCTION public.jobs_log_change();
