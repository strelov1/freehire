-- Per-(user, job) cached LLM fit analysis (see the job-fit-analysis change). One row
-- holds the whole structured verdict for a single candidate against a single job,
-- computed on demand by the three-stage prompt-chain in internal/jobfit and served
-- from cache until it goes stale.
--
-- Staleness is triple-stamped: cv_uploaded_at records the CV the analysis was run
-- against (from users.resume_uploaded_at), job_content_hash records the job text (from
-- jobs.content_hash), and model records the LLM that produced it. The read path
-- (GET /jobs/:slug/fit) compares all three to the live values and reports the cached row
-- stale when any differs, so a re-uploaded CV, a re-ingested job, or an LLM_MODEL upgrade
-- never shows an outdated analysis as current (the model check is the analogue of the
-- enrichment-version and semantic-embedder staleness guards). job_content_hash is
-- nullable: non-board jobs (telegram/habr/geekjob/moderator-created) have none. A stamp
-- absent on BOTH sides counts as unchanged (those jobs are never re-crawled, so a NULL
-- must not force an endless recompute); a hash appearing on one side only is a change.
--
-- FKs cascade: deleting the user or the job removes the analysis (it has no meaning
-- without both). analysis is the sanitized jobfit.Analysis JSON.
--
-- Applied to a fresh volume by initdb after 0008; on an existing prod volume this
-- statement must be run manually BEFORE deploying code that reads the table (no
-- versioned migration runner — see the migrations gotcha in internal/db/AGENTS.md).

CREATE TABLE public.user_job_analysis (
    user_id          bigint      NOT NULL REFERENCES public.users(id) ON DELETE CASCADE,
    job_id           bigint      NOT NULL REFERENCES public.jobs(id) ON DELETE CASCADE,
    analysis         jsonb       NOT NULL,
    model            text        NOT NULL,
    cv_uploaded_at   timestamptz,
    job_content_hash text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, job_id)
);
