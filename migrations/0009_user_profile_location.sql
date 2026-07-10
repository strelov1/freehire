-- Add an optional, structured location & work-mode preferences block to the single
-- user profile. One nullable JSONB value holds the whole block (work_modes, remote
-- reach, base location, relocation targets); NULL means the user set no preferences.
-- Read/written whole by internal/userprofile; not filtered in SQL yet (denormalize to
-- typed columns only when matching needs an index).
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first volume
-- init, so on a persistent volume this ALTER does not auto-apply. The new binary's
-- GetUserProfile/UpsertUserProfile SELECT/RETURN location_preferences, so deploying
-- before running this ALTER makes every profile read and write fail with 42703
-- (undefined column) → 500. Run it first (same as 0005-0008).
ALTER TABLE public.user_profiles
    ADD COLUMN location_preferences jsonb;
