-- Add an optional "skills to avoid" set to the single user profile. Empty by default
-- (unlike skills, which has a cardinality CHECK): a user need not exclude anything.
-- Stored as canonical lowercase tokens, trimmed and deduplicated by internal/userprofile,
-- and — on the frontend — seeded into the jobs filter's skills EXCLUDE set by "Apply my
-- profile" (rendering ?skills_exclude=… → Meili skills != "X").
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first volume
-- init, so on a persistent volume this ALTER does not auto-apply. The new binary's
-- GetUserProfile/UpsertUserProfile SELECT/RETURN excluded_skills, so deploying before
-- running this ALTER makes every profile read and write fail with 42703 (undefined
-- column) → 500. Run it first (same as 0005-0010).
ALTER TABLE public.user_profiles
    ADD COLUMN excluded_skills text[] NOT NULL DEFAULT '{}'::text[];
