-- Employment type and seniority as submitter-stated structured facets, mirroring the
-- pattern migration 0031 already set for skills/regions/cities/work_mode/salary: the
-- submission stores exactly what the submitter entered so it can be echoed back and,
-- on approval, carried onto the minted job as an override that wins over derivation.
ALTER TABLE public.job_submissions
    ADD COLUMN employment_type text DEFAULT ''::text NOT NULL,
    ADD COLUMN seniority text DEFAULT ''::text NOT NULL;
