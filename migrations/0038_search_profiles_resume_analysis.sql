-- The résumé verdict persists only its derived AI layer on the owning search profile:
-- a coherence score, per-gap advice, and the analysis timestamp. The raw résumé text is
-- NEVER stored (the privacy invariant from internal/handler/resume.go) — only this
-- derived JSON. The deterministic part of the verdict (stack match, gaps, unlock) is
-- computed live from the profile's skills/specializations, so it is not stored here.
--
-- Nullable, no default: a profile with no analysis yet is NULL, backward-compatible with
-- existing rows. Like every migration here it applies on fresh volume init and is the
-- schema source for sqlc; existing volumes/prod need a manual apply (the open
-- versioned-migration-runner seam from AGENT.md) — apply this BEFORE rolling the new
-- server binary, which references the new column via the profile fetch query.

ALTER TABLE search_profiles
    ADD COLUMN resume_analysis JSONB;

-- Rollback (inverse), if ever needed:
--   ALTER TABLE search_profiles DROP COLUMN resume_analysis;
