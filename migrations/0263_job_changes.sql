-- The append-only record of a posting opening, closing and reopening, behind the public
-- change feed (GET /api/v1/jobs/changes).
--
-- Why a table and not a read of jobs.closed_at: a reopen CLEARS closed_at (the ingest upsert,
-- a moderator), so the row alone cannot say a posting was closed and came back; a mirror that
-- delisted it would never relist it. A trigger on jobs writes every transition, so each of the
-- closers (ingest sweeps, board-gone, liveness, deadline, moderators, close-chronic-boards)
-- and raw SQL are covered without touching them.
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
    -- jobs.closed_reason at the close; '' for opened and reopened.
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
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- A row inserted already closed (a backfill, a manual import) never was open.
        IF NEW.closed_at IS NULL THEN
            INSERT INTO job_changes (job_id, kind) VALUES (NEW.id, 'opened');
        END IF;
    ELSIF NEW.closed_at IS NOT NULL THEN
        INSERT INTO job_changes (job_id, kind, reason) VALUES (NEW.id, 'closed', NEW.closed_reason);
    ELSE
        INSERT INTO job_changes (job_id, kind) VALUES (NEW.id, 'reopened');
    END IF;
    RETURN NULL;
END;
$$;

CREATE TRIGGER jobs_log_change_ins
    AFTER INSERT ON public.jobs
    FOR EACH ROW
    EXECUTE FUNCTION public.jobs_log_change();

-- Only an open/closed flip logs: a re-close that merely changes the timestamp or reason of an
-- already-closed row is not a transition.
CREATE TRIGGER jobs_log_change_upd
    AFTER UPDATE OF closed_at ON public.jobs
    FOR EACH ROW
    WHEN ((OLD.closed_at IS NULL) <> (NEW.closed_at IS NULL))
    EXECUTE FUNCTION public.jobs_log_change();
