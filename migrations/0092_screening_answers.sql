-- The candidate's own answers to the six screening questions that repeat across ATS
-- application forms and no CV can supply — work authorization, visa sponsorship, desired
-- salary, notice period, willingness to relocate, and 18-or-older (see the
-- screening-answers-profile change). Confirmed against 443k captured apply_forms as the
-- dominant repeat once standard contact fields and demographic/EEO questions are excluded.
--
-- One row per user, six independently nullable columns rather than a jsonb blob: the field
-- set is fixed and small, so naming each fact is simpler to validate and read than a
-- generic key-value store, and null unambiguously means "the candidate has not stated
-- this" for every field on its own — never defaulted, never guessed.
--
-- Not internal/userprofile (search/targeting preferences, a different lifecycle), not
-- internal/experience (an accumulating evidence bank for CV content), not
-- internal/resumeextract (CV-derived — none of these six facts can be derived from a CV).

CREATE TABLE public.screening_answers (
    user_id                  bigint      PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    -- ISO-3166-1 alpha-2 codes, lowercase, validated against internal/location's existing
    -- country dictionary. Empty/null means unstated, not "authorized nowhere".
    authorized_countries     text[],
    visa_sponsorship_needed  boolean,
    desired_salary_amount    integer,
    -- ISO 4217, three uppercase letters. Unlike authorized_countries and
    -- desired_salary_period, there is no closed dictionary for currency in this codebase
    -- (internal/vocab documents it as a deliberately open ISO-standard field) — validated
    -- as well-formed, not dictionary-recognized.
    desired_salary_currency  text,
    -- One of vocab.SalaryPeriodValues (year/month/day/hour) — the same enum
    -- job_submissions.salary_period already uses.
    desired_salary_period    text,
    notice_period_days       integer,
    willing_to_relocate      boolean,
    age_18_or_older          boolean,
    updated_at               timestamptz NOT NULL DEFAULT now()
);

-- No separate index: user_id is already the primary key, and every read is a
-- point lookup by the caller's own id (satisfies TestEveryUserForeignKeyIsIndexed).
