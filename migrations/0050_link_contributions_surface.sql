-- Record which door a link intake came through.
--
-- Every surface that accepts a job link — the website's contribute form, the Telegram bot,
-- the browser extension, the CLI — now enters through one sequence (POST /jobs/resolve). Each
-- of those makes the server fetch a caller-supplied URL, so the channel matters: submitted_by
-- already says WHO, surface says THROUGH WHAT, and repeated or abusive use becomes visible in
-- the data instead of only in the logs.
--
-- Rows recorded before this column get 'unknown', which is also what an absent or unrecognised
-- tag records — an older client is served, never refused. The tag is for our own visibility,
-- not a gate.
--
-- Deliberately NOT touched: the board's uniqueness. Migration 0049 narrowed
-- UNIQUE (source, board) to the live statuses so a rejected board stops claiming its identity
-- forever, and that settles the question this column might otherwise reopen — one row per live
-- board, one reward per board. Paying later contributors for a board already queued buys
-- nothing and invites farming one board from several accounts.
--
-- Applied to a fresh volume by initdb after 0049; on an existing prod volume run this manually
-- (SET ROLE hire) BEFORE deploying code that writes the column.

ALTER TABLE public.link_contributions
    ADD COLUMN surface text DEFAULT 'unknown'::text NOT NULL;

ALTER TABLE public.link_contributions
    ADD CONSTRAINT link_contributions_surface_check
    CHECK ((surface = ANY (ARRAY['web'::text, 'telegram'::text, 'extension'::text, 'cli'::text, 'unknown'::text])));
