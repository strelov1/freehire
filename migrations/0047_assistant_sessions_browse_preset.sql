-- A third assistant preset: `browse` (see the assistant-browse-preset change).
--
-- A browsing session is a conversation held from the browser extension's side
-- panel. Like the other two presets it only selects the system prompt and the tool
-- set — what makes it different is that its agent can read the page the candidate
-- is looking at, through the browser-tool relay the extension already serves.
--
-- 0044 pinned `preset` to ('chat','tailor') with a CHECK, so the column has to be
-- told about the new value before any code can write it. The constraint is dropped
-- and re-added rather than altered: Postgres has no ALTER CONSTRAINT for a CHECK's
-- expression.
--
-- Safe to run against a live database: the rewritten constraint is strictly wider
-- than the one it replaces, so no existing row can violate it, and the validation
-- scan only reads the table. Rolling back means redeploying the old binary — rows
-- already recorded as `browse` would then fail the narrower CHECK, so restore this
-- constraint only after none remain.
--
-- Applied to a fresh volume by initdb after 0046; on an existing prod volume run
-- these statements manually (SET ROLE hire) BEFORE deploying code that writes them.

ALTER TABLE public.assistant_sessions
    DROP CONSTRAINT IF EXISTS assistant_sessions_preset_check;

ALTER TABLE public.assistant_sessions
    ADD CONSTRAINT assistant_sessions_preset_check
    CHECK ((preset = ANY (ARRAY['chat'::text, 'tailor'::text, 'browse'::text])));
