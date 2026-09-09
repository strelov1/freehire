-- The candidate's own answers to employer screening questions, accumulating across
-- applications, so an answer given once serves every later posting that asks the same thing.
--
-- Distinct from screening_answers (0092), which holds SIX typed facts — desired salary as
-- (amount, currency, period), authorized countries as a validated array — and keeps them
-- typed because the product compares them (desired salary against candidate_survey's
-- current income, without conversion) and validates them. Those stay where they are. This
-- table takes the open-ended remainder: the questions employers author, which no fixed
-- column set can anticipate.
--
-- Keyed by TOPIC, not by the question's wording (internal/dict/answertopic). Keying on
-- wording is what fills a bank with near-duplicates and asks the candidate the same thing
-- once per employer.
--
-- question is kept verbatim beside it: a topic alone cannot show what was being asked, and
-- an answer sent to an employer in the candidate's name has to remain auditable against the
-- words they actually read.
--
-- provenance is text validated in Go rather than by a CHECK, matching how
-- screening_answers validates desired_salary_period and how the experience bank handles its
-- own provenance: one answer per repository to "where is an enum enforced", and adding a
-- vocabulary member stays a code change rather than a migration.
CREATE TABLE public.screening_answer_bank (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    topic      text        NOT NULL,
    question   text        NOT NULL,
    answer     text        NOT NULL,
    provenance text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- One answer per topic per candidate: the upsert key, and what makes a second phrasing
    -- of the same question find the existing answer instead of adding another.
    UNIQUE (user_id, topic)
);

-- Every read is "this candidate's answers", ordered for display. The UNIQUE above already
-- covers lookup by (user_id, topic), so this index serves only the list.
CREATE INDEX screening_answer_bank_user_updated_idx
    ON public.screening_answer_bank (user_id, updated_at DESC);
