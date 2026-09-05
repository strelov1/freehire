-- The candidate's self-reported segmentation answers, and the explicit fact of having been
-- through onboarding at all.
--
-- The three survey answers — where they are in their search, what is blocking them, what
-- they earn today — are the questions the wizard could not ask while every one of its steps
-- was a search facet. They are deliberately NOT internal/identity/userprofile: that table is
-- the search profile, it carries CHECK constraints requiring at least one skill and one
-- specialization, and a row cannot even exist for a user who skipped those steps. They are
-- also not internal/ingest/screeninganswers: those six facts are what an EMPLOYER sees on an
-- application form, and none of these three ever is.
--
-- One row per user, independently nullable columns rather than a jsonb blob, for the same
-- reason 0092 gave: the field set is fixed and small, so naming each fact is simpler to
-- validate and read than a generic key-value store, and null unambiguously means "the
-- candidate has not stated this" for every field on its own — never defaulted, never guessed.
CREATE TABLE public.candidate_survey (
    user_id                 bigint      PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    -- One of vocab.JobSearchStageValues. Validated in internal/candidate/survey rather than
    -- by a CHECK here, matching how screening_answers validates desired_salary_period: one
    -- answer per repository to "where is an enum enforced", and adding a vocabulary member
    -- stays a code change rather than a migration.
    job_search_stage        text,
    -- One of vocab.JobChallengeValues. Single-valued on purpose: the question asks for the
    -- BIGGEST difficulty, and a multi-select would collect half the list from everyone and
    -- lose the ranking that made the answer worth having.
    biggest_challenge       text,
    -- Free text, accepted ONLY when biggest_challenge is the vocabulary's `other` member.
    -- Any other pairing is rejected in Go, so a note can never contradict the coded answer.
    biggest_challenge_note  text,
    -- What the candidate earns TODAY, as the same amount/currency/period triple
    -- screening_answers uses for desired salary, so the two figures compare without
    -- conversion. Currency is ISO 4217 (validated as well-formed, not dictionary-recognized
    -- — internal/dict/vocab documents currency as a deliberately open field); period is one
    -- of vocab.SalaryPeriodValues.
    -- squawk-ignore prefer-bigint-over-int -- deliberately the same width as the sibling it is meant to be compared against, screening_answers.desired_salary_amount; a salary in any currency stays orders of magnitude below the int32 ceiling, and a widening here alone would make the two figures differ in Go (int64 vs int32) for no gain
    current_income_amount   integer,
    current_income_currency text,
    current_income_period   text,
    updated_at              timestamptz NOT NULL DEFAULT now()
);

-- No separate index: user_id is already the primary key, and every read is a point lookup
-- by the caller's own id (satisfies TestEveryUserForeignKeyIsIndexed).

-- Whether this account has been walked through onboarding. Until now the answer was
-- INFERRED from "does this account have a CV", which was true while the wizard was about
-- the CV and became false the moment it grew questions a CV cannot answer: every existing
-- account would have skipped all of them forever. Null routes the user into the wizard,
-- non-null never does.
--
-- Nullable with no default, so this is a catalog-only change on a large table.
--
-- Deliberately a timestamp rather than a version counter. Re-asking everyone when a future
-- wave of questions lands is a real future need, but it is not this change's need, and the
-- seam is one more migration when it arrives.
ALTER TABLE public.users
    ADD COLUMN onboarding_completed_at timestamptz;
