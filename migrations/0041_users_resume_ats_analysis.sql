-- The CV ATS-readiness report caches its optional LLM qualitative review per user,
-- keyed to the stored CV (the review is role-independent, so it is computed once per
-- CV and reused across profiles/roles). Only the derived review (content-quality +
-- findings) is stored here — never the raw CV text (the privacy invariant from
-- internal/handler/resume.go). Nullable, no default: a user with no review yet is
-- NULL; it is cleared whenever the CV pointer is set or cleared (SetUserResume /
-- ClearUserResume), so a new CV is never scored with a stale review.
--
-- Like every migration here it applies on fresh volume init and is the schema source
-- for sqlc; existing volumes/prod need a manual apply (the open versioned-migration
-- -runner seam from AGENT.md) — apply this BEFORE rolling the new server binary,
-- which references the new column via the users query (SELECT-based).

ALTER TABLE users
    ADD COLUMN resume_ats_analysis JSONB;
