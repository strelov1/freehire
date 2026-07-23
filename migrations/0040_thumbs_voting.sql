-- Thumbs up/down voting for jobs and companies. Signed-in users cast at most one
-- vote per target; the aggregate is a public quality signal shown to everyone.
--
-- Job votes live as a `vote` column on the existing per-(user, job) user_jobs row
-- (NULL = no vote), alongside the save/apply/dismiss marks. Company votes get their
-- own per-(user, company) table — companies had no per-user interaction row before
-- (following is expressed via saved-searches, not a row).
--
-- Each job and company carries materialized upvote_count/downvote_count on its own
-- row (like view_count/applied_count), served straight from the row on every read.
-- The vote write recomputes the affected target's two counters in the same
-- transaction, so a reader never sees a vote without its counter. The partial index
-- on user_jobs(job_id) and the index on company_votes(company_slug) keep that
-- single-target recompute a cheap indexed count.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first volume
-- init, so on a persistent volume this does not auto-apply. The new binary's vote
-- writes and job/company reads SELECT/RETURN these columns, so deploying before
-- running this makes those queries fail with 42703 (undefined column) → 500. Run it
-- first (same as 0005-0010, 0039).

ALTER TABLE public.user_jobs
    ADD COLUMN vote smallint,
    ADD CONSTRAINT user_jobs_vote_check CHECK (vote IN (-1, 1));

ALTER TABLE public.jobs
    ADD COLUMN upvote_count   integer NOT NULL DEFAULT 0,
    ADD COLUMN downvote_count integer NOT NULL DEFAULT 0;

ALTER TABLE public.companies
    ADD COLUMN upvote_count   integer NOT NULL DEFAULT 0,
    ADD COLUMN downvote_count integer NOT NULL DEFAULT 0;

CREATE TABLE public.company_votes (
    user_id      bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    company_slug text        NOT NULL REFERENCES public.companies (slug) ON DELETE CASCADE,
    vote         smallint    NOT NULL CHECK (vote IN (-1, 1)),
    created_at   timestamp with time zone NOT NULL DEFAULT now(),
    updated_at   timestamp with time zone NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, company_slug)
);

-- Scope the single-target counter recompute to an indexed count. Votes are a small
-- subset of user_jobs interactions, so the job-side index is partial.
CREATE INDEX user_jobs_job_id_vote_idx ON public.user_jobs (job_id) WHERE vote IS NOT NULL;
CREATE INDEX company_votes_company_slug_idx ON public.company_votes (company_slug);
