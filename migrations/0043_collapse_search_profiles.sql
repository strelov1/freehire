-- Collapse the per-user search-profile *collection* into a single per-user profile.
-- A person has one professional identity; the many-named-profiles model (surrogate id,
-- per-user unique name, 50-cap, picker) carried complexity nobody needs. The table
-- becomes keyed by user_id (the 1:1 invariant), loses `name`, and is renamed
-- `user_profiles`. Specializations/skills and their CHECK backstops are unchanged.
--
-- Destructive by design: a user with several old profiles keeps only their
-- most-recently-updated one (tiebroken by id); the rest are discarded. The feature is
-- MVP-stage with negligible data, so this is acceptable (agreed at design time).
--
-- Like every migration here it applies on fresh volume init and is the schema source for
-- sqlc; existing volumes/prod need a MANUAL apply (the open versioned-migration-runner
-- seam from AGENT.md) — apply this BEFORE rolling the new server binary, whose queries
-- reference `user_profiles` and no longer select `id`/`name`.

BEGIN;

-- 1. Keep only each user's most-recently-updated row (tiebreak by id); drop the rest.
DELETE FROM search_profiles sp
USING search_profiles keep
WHERE sp.user_id = keep.user_id
  AND (keep.updated_at, keep.id) > (sp.updated_at, sp.id);

-- 2. The picker index is redundant once user_id is unique.
DROP INDEX IF EXISTS search_profiles_user_updated_idx;

-- 3. Drop `name`; this cascades its CHECK and the UNIQUE (user_id, name) constraint.
ALTER TABLE search_profiles DROP COLUMN name;

-- 4. Drop the surrogate `id` (cascades its PK) and make user_id the primary key — the
--    structural expression of "at most one profile per user".
ALTER TABLE search_profiles DROP COLUMN id;
ALTER TABLE search_profiles ADD PRIMARY KEY (user_id);

-- 5. Tidy the surviving CHECK name and rename the table to match the new concept.
ALTER TABLE search_profiles
    RENAME CONSTRAINT search_profiles_specializations_card_chk
    TO user_profiles_specializations_card_chk;
ALTER TABLE search_profiles RENAME TO user_profiles;

COMMIT;

-- Rollback (inverse), if ever needed — note discarded rows are NOT recoverable:
--   ALTER TABLE user_profiles RENAME TO search_profiles;
--   ALTER TABLE search_profiles
--       RENAME CONSTRAINT user_profiles_specializations_card_chk
--       TO search_profiles_specializations_card_chk;
--   ALTER TABLE search_profiles DROP CONSTRAINT search_profiles_pkey;
--   ALTER TABLE search_profiles ADD COLUMN id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY;
--   ALTER TABLE search_profiles ADD COLUMN name TEXT NOT NULL DEFAULT '';
--   ALTER TABLE search_profiles ADD CONSTRAINT search_profiles_user_id_name_key UNIQUE (user_id, name);
--   CREATE INDEX search_profiles_user_updated_idx ON search_profiles (user_id, updated_at DESC);
