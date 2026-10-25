-- Admit the 'apply' preset (see the auto-apply-chat change): one conversation held against
-- one application, in which the assistant narrates an automatic application's progress and
-- takes the candidate's answers and decisions in free text. It binds like 'interview' and
-- 'debrief' — a vacancy and no CV.
--
-- The list is rewritten in full, as in 0048/0049/0062/0069: Postgres has no ALTER CONSTRAINT
-- for a CHECK's expression, so every value added before carries forward. The new constraint
-- is strictly wider than the one it replaces, so no existing row can violate it. The explicit
-- BEGIN/COMMIT closes the window between DROP and ADD for initdb and a manual application;
-- see 0069 for what that means under cmd/migrate.
--
-- Rolling back means redeploying the old binary; restore the narrower constraint only once
-- no 'apply' rows remain. Ships alone for that reason. Apply BEFORE code that writes the value.

-- The explicit transaction is deliberate (see above), so the nesting warning does not apply.
-- squawk-ignore transaction-nesting
BEGIN;

ALTER TABLE public.assistant_sessions
    DROP CONSTRAINT IF EXISTS assistant_sessions_preset_check;

ALTER TABLE public.assistant_sessions
    -- squawk-ignore constraint-missing-not-valid -- strictly wider than the constraint it replaces, so the validating scan cannot fail
    ADD CONSTRAINT assistant_sessions_preset_check
    CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text, 'profile'::text, 'browse'::text, 'interview'::text, 'debrief'::text, 'apply'::text])));

-- squawk-ignore transaction-nesting
COMMIT;
