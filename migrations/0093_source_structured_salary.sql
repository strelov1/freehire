-- Structured, ATS-provided salary on jobs — distinct from the LLM-derived salary in the
-- enrichment JSONB and from the moderator-authoritative salary_*_manual columns (0031).
-- Lever, Ashby, and Recruitee all expose a genuine structured salary field in their public
-- APIs (confirmed live 2026-08-14: Lever's salaryRange, Ashby's compensation.compensationTiers
-- behind ?includeCompensation=true, Recruitee's salary). An adapter sets these only when the
-- platform states the value in a structured field — the same contract as
-- sources.Job.EmploymentType/Countries, never a guess from free text.
--
-- NULL salary_min_source means "the source stated none". SetJobEnrichment overlays it into the
-- enrichment JSONB ahead of the LLM's own payload but behind the manual columns, so the
-- effective precedence is manual > source > LLM-guessed.
ALTER TABLE public.jobs
    ADD COLUMN salary_min_source integer,
    ADD COLUMN salary_max_source integer,
    ADD COLUMN salary_currency_source text DEFAULT ''::text NOT NULL,
    ADD COLUMN salary_period_source text DEFAULT ''::text NOT NULL;
