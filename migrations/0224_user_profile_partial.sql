-- Persist a profile as it is filled in: roles and skills may be empty.
-- Deploy before code that initializes profiles and autosaves individual fields.
ALTER TABLE public.user_profiles
    DROP CONSTRAINT IF EXISTS search_profiles_skills_check,
    DROP CONSTRAINT IF EXISTS user_profiles_specializations_card_chk;

ALTER TABLE public.user_profiles
    -- squawk-ignore constraint-missing-not-valid -- existing rows satisfy the stricter 1..10 bound; this small table can be validated immediately
    ADD CONSTRAINT user_profiles_specializations_card_chk
    CHECK (cardinality(specializations) <= 10);
