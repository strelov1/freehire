-- Company feedback: a signed-in user's 1-5 star rating + a closed category +
-- free text on a company, shown under their site-wide pseudonymous persona (the
-- same community_personas handle internal/community's discussion threads use —
-- so a review of an employer can't come back on the reviewer, and a user has one
-- identity everywhere it's shown, not a second one minted here).
--
-- One row per (user, company); NULL user_id (deleted account) is exempt from the
-- uniqueness so an orphaned row never blocks a re-review by someone else. Content
-- outlives its author's account, de-authored, the same way thread_replies does.
--
-- companies.feedback_count/feedback_rating_avg are materialized counters, same
-- pattern as upvote_count/downvote_count (0040_thumbs_voting.sql): recomputed in
-- the same transaction as the write, in Go, so a reader never sees a rating
-- without its average.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first
-- volume init, so on a persistent volume this does not auto-apply. The new
-- binary's feedback writes/reads and the company read SELECT these columns, so
-- deploying before running this makes those queries fail with 42703 (undefined
-- column) / 42P01 (undefined table) → 500. Run it first (same as 0005-0010,
-- 0039-0041).

ALTER TABLE public.companies
    ADD COLUMN feedback_count integer NOT NULL DEFAULT 0,
    ADD COLUMN feedback_rating_avg real;

CREATE TABLE public.company_feedback (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id bigint REFERENCES public.users (id) ON DELETE SET NULL,
    company_slug text NOT NULL REFERENCES public.companies (slug) ON DELETE CASCADE,
    rating smallint NOT NULL,
    feedback_type text NOT NULL,
    body text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT company_feedback_rating_check CHECK (rating BETWEEN 1 AND 5),
    CONSTRAINT company_feedback_type_check CHECK (feedback_type = ANY (ARRAY[
        'interview', 'culture', 'compensation', 'management',
        'work_life_balance', 'career_growth', 'other'
    ]::text[]))
);

-- One feedback per user per company; a NULL user_id (deleted account) is exempt.
CREATE UNIQUE INDEX company_feedback_user_company_uniq_idx
    ON public.company_feedback (user_id, company_slug) WHERE user_id IS NOT NULL;

-- The subject listing: a company's feedback, newest first.
CREATE INDEX company_feedback_company_slug_idx
    ON public.company_feedback (company_slug, created_at DESC, id DESC);
