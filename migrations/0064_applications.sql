-- The application as a record of what a person did, rather than a property of a catalogue row.
--
-- user_jobs is keyed (user_id, job_id) and holds two unrelated things under that one key: marks
-- on a catalogue row (viewed, saved, dismissed, voted), where a job_id is essential, and an
-- application (applied_at, stage, notes, followed_up_at), where the posting is merely where the
-- person found the role. Both cascade from jobs, so cmd/prune — the only hard-delete path, and
-- one that has already removed 1.6M rows on production — takes the application with the posting.
-- PruneJobs weighs that trade in a comment about a *saved* job, which is a bookmark; the same
-- cascade silently covered applications, which are not.
--
-- The same missing identity costs the public company response rate. application_events (0062)
-- denormalises company_slug onto every event so a removal cannot orphan it, but pairs a reply
-- back to its application through (user_id, job_id) — and that job_id is ON DELETE SET NULL. Once
-- both are cleared the pair reduces to NULL = NULL, which is never true: the applied event stays
-- in the denominator, its employer_reply drops out of the numerator, and an employer that
-- answered is served as silent. That is the distortion 0062 was written to remove, arriving by a
-- different route.
--
-- Applied to a fresh volume by initdb after 0063; on an existing prod volume run this manually
-- (SET ROLE hire) BEFORE deploying code that reads it. Additive — the backfill is its own pass.

CREATE TABLE IF NOT EXISTS applications (
    id             bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The employer and the role as they were when the person applied, stored here rather than
    -- read through the posting, because the posting is the part that can disappear. Not
    -- refreshed when the posting is later edited: a rename is rare, and an application should
    -- record the employer it was made to.
    company_slug   text        NOT NULL DEFAULT '',
    role_title     text        NOT NULL DEFAULT '',
    -- Where the person found the role, while we still have it. ON DELETE SET NULL, matching
    -- emails.job_id (0020) and application_events.job_id (0062): the link clears, the record
    -- stands. This column may legitimately become unknown; which application a fact belongs to
    -- may not, which is why nothing correlates on it.
    job_id         bigint      REFERENCES jobs (id) ON DELETE SET NULL,
    applied_at     timestamptz NOT NULL,
    -- The working state of the application. stage draws on the controlled vocabulary in
    -- internal/userjob; notes is the candidate's own text, and is the reason the ledger cannot
    -- stand in for this table — the ledger is content-free by design.
    stage          text,
    notes          text,
    followed_up_at timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- Today's "at most one application per (user, posting)" rule, carried across the move. Partial,
-- so it binds only while a posting is named: two applications to one employer with no posting are
-- two different roles, and a unique index over a nullable column would not have said so.
CREATE UNIQUE INDEX IF NOT EXISTS applications_user_job_idx
    ON applications (user_id, job_id) WHERE job_id IS NOT NULL;

-- The board read (a user's applications, newest first) and the account-deletion cascade. The
-- unique index above also leads with user_id but excludes unlinked applications, so the cascade
-- would still have to scan for them.
CREATE INDEX IF NOT EXISTS applications_user_applied_idx
    ON applications (user_id, applied_at DESC);

-- The per-company rollup groups by this and, once the correlation moves, joins nothing.
CREATE INDEX IF NOT EXISTS applications_company_idx
    ON applications (company_slug) WHERE company_slug <> '';

-- The identity the ledger correlates on. CASCADE, unlike every other reference in this migration:
-- an event belongs to its application, and no scheduled campaign deletes an application — only
-- the person removing their own record, whose events should go with it.
ALTER TABLE public.application_events
    ADD COLUMN IF NOT EXISTS application_id bigint REFERENCES applications (id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS application_events_application_idx
    ON public.application_events (application_id) WHERE application_id IS NOT NULL;

-- Mail hangs off the application, not off inventory, so a removal cannot detach a thread from an
-- application that still exists. suggested_job_id deliberately keeps pointing at jobs: a
-- suggestion is "this mail may concern that posting", made before any application exists.
ALTER TABLE public.emails
    ADD COLUMN IF NOT EXISTS application_id bigint REFERENCES applications (id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS emails_application_id_idx
    ON public.emails (application_id) WHERE application_id IS NOT NULL;
