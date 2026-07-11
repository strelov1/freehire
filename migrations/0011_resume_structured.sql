-- LLM-derived structured résumé for the profile view and as pre-normalized fit input
-- (see the resume-structured-profile change). resume_structured holds the sanitized,
-- typed structure (contacts, summary, experience, education, languages, links, years);
-- resume_structured_model records the LLM identity that produced it (parity with
-- resume_embedding_model — a model change lets a future backfill find stale rows);
-- resume_structured_uploaded_at stamps the résumé upload time the structure was derived
-- from. The read surface serves the structure only when that stamp still equals
-- users.resume_uploaded_at, so a structure derived from a superseded CV is treated as
-- absent (self-healing on the next successful extraction) — no synchronous clear on
-- re-upload. Only derived data is stored — never the raw CV text.
--
-- Applied to a fresh volume by initdb after the earlier migrations; on an existing prod
-- volume this ALTER must be run manually BEFORE deploying code that reads the columns.
ALTER TABLE public.users
    ADD COLUMN resume_structured jsonb,
    ADD COLUMN resume_structured_model text,
    ADD COLUMN resume_structured_uploaded_at timestamp with time zone;
