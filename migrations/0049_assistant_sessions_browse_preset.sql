-- Admit the 'browse' preset (see the assistant-browse-preset change).
--
-- A browsing session is a conversation held from the browser extension's side
-- panel. Like every other preset it only selects the system prompt and the tool
-- set — what makes it different is that its agent can read the page the candidate
-- is looking at, through the browser-tool relay the extension already serves.
--
-- 0044 pinned the preset vocabulary in a CHECK and 0048 widened it for 'profile';
-- this widens it again. The list is rewritten in full rather than appended to,
-- because Postgres has no ALTER CONSTRAINT for a CHECK's expression — which means
-- every such migration must carry forward every value added before it. Adding
-- 'browse' while forgetting 'profile' would fail every profile session with a 23514.
--
-- Two of these were written in parallel: this change and experience-bank overlapped,
-- and 0048 was authored against a vocabulary that did not yet know about 'browse'.
-- Applying them in file order reaches the right end state; applying them out of
-- order does not.
--
-- Wrapped in a transaction so the column is never briefly unconstrained: between
-- the DROP and the ADD there is a window in which any value would be accepted.
--
-- Safe to run against a live database at this table's size: the rewritten
-- constraint is strictly wider than the one it replaces, so no existing row can
-- violate it. The ADD does hold ACCESS EXCLUSIVE while it validates, which is
-- nothing here and would not be on a large table — there the shape is
-- `ADD ... NOT VALID` followed by `VALIDATE CONSTRAINT`.
--
-- Rolling back means redeploying the old binary — rows already recorded as `browse`
-- would then fail the narrower CHECK, so restore the previous constraint only after
-- none remain.
--
-- Applied to a fresh volume by initdb after 0048; on an existing prod volume run
-- these statements manually (SET ROLE hire) BEFORE deploying code that writes them.

BEGIN;

ALTER TABLE public.assistant_sessions
    DROP CONSTRAINT IF EXISTS assistant_sessions_preset_check;

ALTER TABLE public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_preset_check
    CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text, 'profile'::text, 'browse'::text])));

COMMIT;
