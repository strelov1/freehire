-- Candidate-reported facts about how a company hires. The first — and today the
-- only — one is `ai_interview`: an employer screened the reporter with an AI
-- interviewer. A candidate cannot learn this before they are already inside one,
-- so nothing in the catalogue carries it and every candidate rediscovers it alone.
--
-- The evidence is COMPANY-scoped, not job-scoped, and that is the whole point.
-- An AI interviewer is a property of an employer's hiring process; filed per
-- posting the signal never reaches useful coverage (micro1 alone has 203 open
-- postings, each needing its own reporter before its own card shows anything).
--
-- `kind` is a CHECK list rather than a table per signal because the family is
-- real and named — an unpaid test task, government ID demanded before an offer —
-- and three tables differing only in their name would be the worse answer. The
-- list is a code constant mirrored here: it decides what the badge renders and
-- what the search facet declares, so it is not configuration.
--
-- Retraction rather than deletion, for the reason ghost_reports gives
-- (0053_ghost_job_signal.sql): withdrawing is how the signal self-heals when an
-- employer changes practice, and keeping the row preserves the uniqueness bound
-- so a retraction cannot be used to file repeatedly. A re-file clears
-- retracted_at on the existing row rather than inserting a second.
--
-- user_id is NOT NULL and cascades, unlike company_feedback's ON DELETE SET NULL
-- (0088_company_feedback.sql): feedback is authored content that outlives its
-- author de-authored, while this is a countable claim that means nothing without
-- a claimant.
--
-- companies.ai_interview_reports is a materialized counter, same pattern as
-- feedback_count and upvote_count/downvote_count (0040_thumbs_voting.sql):
-- recomputed in the same transaction as the write, in Go, so a reader never sees
-- the label without the count that qualifies it. There is exactly one writer and
-- no background job recomputes it.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first
-- volume init, so on a persistent volume this does not auto-apply. The new
-- binary's report writes and every company/job read SELECT these, so deploying
-- before running this makes those queries fail with 42703 (undefined column) /
-- 42P01 (undefined table) → 500. Run it first (same as 0005-0010, 0039-0041,
-- 0088).

CREATE TABLE public.company_process_reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    company_slug text NOT NULL REFERENCES public.companies (slug) ON DELETE CASCADE,
    kind text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    -- Set on withdrawal; the row stays. NULL means the report counts.
    retracted_at timestamp with time zone,
    CONSTRAINT company_process_reports_kind_check CHECK (kind = ANY (ARRAY[
        'ai_interview'
    ]::text[])),
    -- The abuse bound, enforced here rather than in the service so no code path
    -- can file twice.
    CONSTRAINT company_process_reports_user_company_kind_key UNIQUE (user_id, company_slug, kind)
);

-- The counting read: un-retracted rows for one company and kind. The UNIQUE
-- constraint's index leads with user_id, so it cannot serve this.
CREATE INDEX company_process_reports_company_kind_live_idx
    ON public.company_process_reports (company_slug, kind)
    WHERE retracted_at IS NULL;

-- NOT NULL with a DEFAULT because a NULL count would render as an absent label
-- rather than as zero; on PG11+ a non-volatile default is a metadata-only change.
ALTER TABLE public.companies
    -- squawk-ignore prefer-bigint-over-int -- mirrors its neighbours on this table (job_count, feedback_count, upvote_count are all integer); one company reaching 2.1 billion candidate reports is not a scenario
    ADD COLUMN ai_interview_reports integer NOT NULL DEFAULT 0;
