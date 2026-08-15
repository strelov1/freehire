-- The language every free-text field of a cached fit analysis was written in — the
-- fourth staleness stamp alongside model/cv_uploaded_at/job_content_hash (see 0009).
-- Without it, a candidate who switches their profile language (freehire#1836) would
-- keep being served an analysis whose comments, strengths, gaps and recommendation
-- were written in their OLD language until the CV or the job text happened to change
-- for an unrelated reason.
--
-- DEFAULT 'en' is a non-volatile constant, so this is metadata-only (PG11+) — no scan,
-- no lock beyond the brief one every DDL statement takes. Every row cached before this
-- column existed was in fact written in English (the only language matchanalysis ever
-- targeted before freehire#1837), so the default is not a guess; it is the historical
-- truth for every existing row.
ALTER TABLE public.user_job_analysis
    ADD COLUMN language text NOT NULL DEFAULT 'en';
