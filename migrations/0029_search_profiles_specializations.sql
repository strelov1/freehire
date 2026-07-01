-- A search profile now captures several specializations, not one: people combine roles
-- (e.g. "Go backend" and "DevOps"). This converts the single `specialization TEXT` column
-- into a `specializations TEXT[]` set, mirroring the existing `skills TEXT[]` (same
-- normalization and CHECK-as-backstop shape). Each existing row's single value becomes a
-- one-element set, so the change is lossless. Like every migration here it applies on fresh
-- volume init and is the schema source for sqlc; existing volumes/prod need a manual apply
-- (the versioned-migration-runner seam from AGENT.md remains open) — apply this BEFORE
-- rolling the new server binary, which references the new column.

ALTER TABLE search_profiles
    ADD COLUMN specializations TEXT[] NOT NULL DEFAULT '{}';

-- Backfill each existing single specialization into a one-element set.
UPDATE search_profiles
SET specializations = ARRAY[specialization];

-- Drop the seeding default now that every row is populated; the set is validated (each a
-- known category) in the service and bounded here as a backstop.
ALTER TABLE search_profiles
    ALTER COLUMN specializations DROP DEFAULT;

ALTER TABLE search_profiles
    ADD CONSTRAINT search_profiles_specializations_card_chk
        CHECK (cardinality(specializations) BETWEEN 1 AND 5);

ALTER TABLE search_profiles
    DROP COLUMN specialization;

-- Rollback (inverse), if ever needed:
--   ALTER TABLE search_profiles ADD COLUMN specialization TEXT NOT NULL DEFAULT '';
--   UPDATE search_profiles SET specialization = specializations[1];
--   ALTER TABLE search_profiles ALTER COLUMN specialization DROP DEFAULT;
--   ALTER TABLE search_profiles ADD CONSTRAINT search_profiles_specialization_chk
--       CHECK (length(trim(specialization)) > 0);
--   ALTER TABLE search_profiles DROP CONSTRAINT search_profiles_specializations_card_chk;
--   ALTER TABLE search_profiles DROP COLUMN specializations;
