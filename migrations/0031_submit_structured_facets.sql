-- Structured facets on job submissions and an authoritative manual salary on jobs.
--
-- A submitter (recruiter) can now state a vacancy's skills, geography, work mode, and salary
-- explicitly on the /submit form instead of leaving them to dictionary/LLM derivation. The
-- submission stores exactly what was entered; on approval the explicit facets seed the minted
-- job as overrides that win over derivation, and the salary becomes an authoritative manual
-- salary on the job that the enrichment pass never displaces.

-- job_submissions: what the submitter explicitly stated. Arrays follow the jobs-table
-- convention (NOT NULL default '{}'); salary_min/max are nullable so "not stated" is
-- distinguishable from zero.
ALTER TABLE public.job_submissions
    ADD COLUMN skills text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN regions text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN cities text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN work_mode text DEFAULT ''::text NOT NULL,
    ADD COLUMN salary_min integer,
    ADD COLUMN salary_max integer,
    ADD COLUMN salary_currency text DEFAULT ''::text NOT NULL,
    ADD COLUMN salary_period text DEFAULT ''::text NOT NULL;

-- jobs: the authoritative manual salary. NULL salary_min_manual means "no manual salary";
-- when present it wins over the LLM-enriched salary in the effective projection.
ALTER TABLE public.jobs
    ADD COLUMN salary_min_manual integer,
    ADD COLUMN salary_max_manual integer,
    ADD COLUMN salary_currency_manual text DEFAULT ''::text NOT NULL,
    ADD COLUMN salary_period_manual text DEFAULT ''::text NOT NULL;
