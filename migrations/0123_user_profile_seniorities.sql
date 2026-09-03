-- Add an optional set of desired seniority levels to the single user profile, mirroring
-- excluded_skills: empty by default, no cardinality CHECK (unlike specializations/skills,
-- a profile need not state a level). Values are validated by internal/identity/userprofile
-- against vocab.SeniorityValues and stored as-is (the vocabulary is already canonical, no
-- casing/trimming needed the way free-text skills require).
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first volume
-- init, so on a persistent volume this ALTER does not auto-apply. The new binary's
-- GetUserProfile/UpsertUserProfile SELECT/RETURN seniorities, so deploying before
-- running this ALTER makes every profile read and write fail with 42703 (undefined
-- column) → 500. Run it first (same as 0039).
ALTER TABLE public.user_profiles
    ADD COLUMN seniorities text[] NOT NULL DEFAULT '{}'::text[];
