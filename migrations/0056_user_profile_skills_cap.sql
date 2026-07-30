-- Bound the cardinality of the user profile's skill sets, the backstop for the cap
-- internal/userprofile now enforces (maxSkills = 100).
--
-- Why it matters beyond storage: the coverage verdict turns the wanted set into one
-- `skills != "<skill>"` AND group per element (search.AndNotSkills), so the list is a
-- multiplier on every later read of the profile — against the same Meilisearch index that
-- serves public /jobs/search. specializations has carried a cardinality CHECK since 0001 and
-- the stateless coverage endpoint has capped a supplied list at 100 all along; the profile —
-- the one path that PERSISTS the list, and so lets one write amplify unboundedly many cheap
-- reads — was the entry point without a cap.
--
-- NOT VALID is deliberate. It enforces the bound on every INSERT and UPDATE from here on
-- (which is all the backstop is for) while skipping the validation scan of existing rows, so
-- the statement takes no lengthy lock and cannot fail the migration on a legacy row written
-- before the application cap existed. Such a row can only shrink: the service normalizes
-- every write to at most 100 skills before SQL sees it. To promote it later, once prod is
-- known clean: ALTER TABLE public.user_profiles VALIDATE CONSTRAINT <name>.
--
-- Applied to a fresh volume by initdb after 0055; on an existing prod volume run manually
-- (SET ROLE hire). Safe in either order relative to the image — the new binary writes lists
-- that already satisfy the bound, and the old binary is unaffected by a constraint it never
-- crosses.
ALTER TABLE public.user_profiles
    ADD CONSTRAINT user_profiles_skills_card_chk
    CHECK (cardinality(skills) <= 100) NOT VALID;

ALTER TABLE public.user_profiles
    ADD CONSTRAINT user_profiles_excluded_skills_card_chk
    CHECK (cardinality(excluded_skills) <= 100) NOT VALID;
