-- Admit the 'interview' preset (see the interview-rehearsal-preset change).
--
-- A rehearsal session is a mock interview held against one application. Like every
-- other preset it only selects the system prompt and the tool set; what makes it
-- different is its binding — a vacancy and no CV, because it reads a CV and the fit
-- analysis but edits neither.
--
-- 0044 pinned the vocabulary in a CHECK, 0048 widened it for 'profile' and 0049 for
-- 'browse'; this widens it again. The list is rewritten in full rather than appended
-- to, because Postgres has no ALTER CONSTRAINT for a CHECK's expression — so every
-- such migration carries forward every value added before it. Dropping one while
-- adding this would fail every session of that preset with a 23514.
--
-- Wrapped in a transaction so the column is never briefly unconstrained: between the
-- DROP and the ADD there is a window in which any value would be accepted. That
-- window is the whole risk here, and it is why the rewrite is not two deploys. The
-- explicit BEGIN/COMMIT is for the two paths that have no transaction of their own —
-- initdb, and the manual prod application described below.
--
-- Under cmd/migrate it costs something, and the same is true of 0044/0048/0049: the
-- runner already wraps each file (internal/migrate/migrate.go), so the COMMIT here
-- ends the runner's transaction early and the schema_migrations INSERT lands outside
-- it. A crash in that window leaves the constraint replaced but unrecorded, and the
-- re-run then fails with a 42710 on ADD CONSTRAINT. The remedy is to record the
-- version by hand, not to re-apply.
--
-- Safe against a live database: the new constraint is strictly wider than the one it
-- replaces, so no existing row can violate it. The ADD holds ACCESS EXCLUSIVE while
-- it validates, which is nothing at this table's size and would not be on a large one
-- — there the shape is `ADD ... NOT VALID` followed by `VALIDATE CONSTRAINT`.
--
-- Rolling back means redeploying the old binary; rows already recorded as 'interview'
-- would then fail the narrower CHECK, so restore the previous constraint only once
-- none remain.
--
-- Applied to a fresh volume by initdb after 0061; on an existing prod volume run these
-- statements manually (SET ROLE hire) BEFORE deploying code that writes them.

BEGIN;

ALTER TABLE public.assistant_sessions
    DROP CONSTRAINT IF EXISTS assistant_sessions_preset_check;

ALTER TABLE public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_preset_check
    CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text, 'profile'::text, 'browse'::text, 'interview'::text])));

COMMIT;
