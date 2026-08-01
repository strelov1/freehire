-- Admit the 'debrief' preset (see the interview-debrief change).
--
-- A debrief reviews an interview that has already happened, held against one
-- application. It shares the rehearsal's binding — a vacancy and no CV — and its whole
-- tool set; what differs is the prompt, and one rule inside it that inverts. A rehearsal
-- must guard against banking an improvisation; a debrief exists to bank a recollection.
--
-- 0044 pinned the vocabulary in a CHECK and 0048/0049/0062 widened it for 'profile',
-- 'browse' and 'interview'. As in those, the list is rewritten in full rather than
-- appended to: Postgres has no ALTER CONSTRAINT for a CHECK's expression, so every such
-- migration carries forward every value added before it. Dropping one while adding this
-- would fail every session of that preset with a 23514.
--
-- The explicit BEGIN/COMMIT closes the window between the DROP and the ADD in which the
-- column would accept anything. It matters for the two paths that have no transaction of
-- their own — initdb, and a manual application on prod. Under cmd/migrate it ends the
-- runner's own transaction early, so a crash between COMMIT and the schema_migrations
-- INSERT leaves the constraint replaced but unrecorded; the remedy is to record the
-- version by hand, not to re-apply, which would fail with 42710 on ADD CONSTRAINT.
--
-- Safe against a live database: the new constraint is strictly wider than the one it
-- replaces, so no existing row can violate it.
--
-- Rolling back means redeploying the old binary; rows already recorded as 'debrief'
-- would then fail the narrower CHECK, so restore the previous constraint only once none
-- remain. This ships in its own migration for that reason — a rollback that has to
-- delete rows should not be entangled with anything else.
--
-- Apply BEFORE deploying code that writes the value.

BEGIN;

ALTER TABLE public.assistant_sessions
    DROP CONSTRAINT IF EXISTS assistant_sessions_preset_check;

ALTER TABLE public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_preset_check
    CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text, 'profile'::text, 'browse'::text, 'interview'::text, 'debrief'::text])));

COMMIT;
