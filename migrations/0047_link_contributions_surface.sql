-- Unified link intake: every surface that accepts a job link (the website, the Telegram
-- bot, the browser extension, the CLI) now enters through one sequence — catalog lookup,
-- link-source import, then this queue. Two consequences for the table.
--
-- 1. The board stops being the row's identity.
--
-- Recording only happens for a board we do NOT already crawl, but it now happens even when
-- the vacancy itself was imported successfully — otherwise importing one posting hides the
-- board, and the other twenty vacancies on it, from onboarding. Several links to one board
-- are therefore legitimate rows, each with its own submitter.
--
-- UNIQUE (source, board) has to go, and not only because it blocks the second row: it is
-- actively a reward trap today. Two Microsoft links resolved to one Eightfold board and the
-- promote transaction aborted on the duplicate key, so a second person contributing an
-- already-recorded employer mechanically cannot be rewarded. "One board, one reward" moves
-- into the insert query (a NOT EXISTS over the same table, in the same transaction), where
-- it is a policy we can state rather than a constraint we trip over.
--
-- The index is re-created NON-unique: that reward check and the onboarding queue both look
-- rows up by (source, board), and dropping the constraint would otherwise leave them with a
-- sequential scan.
--
-- The review-queue dedup (UNIQUE url WHERE source IS NULL, migration 0037) is deliberately
-- left alone — an unrecognised link still belongs in the queue at most once.
--
-- 2. Every intake records where it came from.
--
-- submitted_by already says who; surface says through which door, so repeated or abusive
-- use is visible in the data instead of only in logs. Existing rows predate the tag and get
-- 'unknown', which is also what an absent or unrecognised tag records — an older client is
-- served, not refused.
--
-- Applied to a fresh volume by initdb after 0046; on an existing prod volume run these
-- statements manually (SET ROLE hire) BEFORE deploying code that writes the column.

ALTER TABLE public.link_contributions
    DROP CONSTRAINT link_contributions_source_board_key;

CREATE INDEX link_contributions_source_board_idx
    ON public.link_contributions USING btree (source, board);

ALTER TABLE public.link_contributions
    ADD COLUMN surface text DEFAULT 'unknown'::text NOT NULL;

ALTER TABLE public.link_contributions
    ADD CONSTRAINT link_contributions_surface_check
    CHECK ((surface = ANY (ARRAY['web'::text, 'telegram'::text, 'extension'::text, 'cli'::text, 'unknown'::text])));
