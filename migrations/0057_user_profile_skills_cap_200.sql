-- Raise the user profile's skill-set cardinality bound from 100 to 200, matching
-- internal/userprofile's maxSkills.
--
-- 0056_user_profile_skills_cap picked 100 for symmetry with the stateless coverage endpoint,
-- on the assumption that a real profile lists well under it. Prod says otherwise: of 131
-- profiles, 89% list 50 skills or fewer, but the largest lists 90 — ten short of the bound.
-- The CV-autofill path unions extracted skills into whatever the form already holds, so that
-- user could cross it and be refused a save. The purpose of the bound is to stop a list of
-- 10^5 from expanding into a Meilisearch filter of 10^5 AND groups on every read; 200 serves
-- that just as well as 100, and 90 groups already run against prod's index today.
--
-- Dropped and re-added rather than altered, because Postgres has no ALTER CONSTRAINT for a
-- CHECK expression. Kept NOT VALID for the reason 0056 gives: the bound is enforced on every
-- INSERT and UPDATE from here on, which is all a backstop is for, without a validation scan.
-- Widening a bound can never reject a row the narrower one admitted, so this cannot fail on
-- existing data. The table is small (131 rows) and the lock is momentary.
ALTER TABLE public.user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_skills_card_chk;

ALTER TABLE public.user_profiles
    ADD CONSTRAINT user_profiles_skills_card_chk
    CHECK (cardinality(skills) <= 200) NOT VALID;

ALTER TABLE public.user_profiles
    DROP CONSTRAINT IF EXISTS user_profiles_excluded_skills_card_chk;

ALTER TABLE public.user_profiles
    ADD CONSTRAINT user_profiles_excluded_skills_card_chk
    CHECK (cardinality(excluded_skills) <= 200) NOT VALID;
