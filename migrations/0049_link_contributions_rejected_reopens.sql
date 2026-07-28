-- A rejected board contribution must not claim the board's identity forever.
--
-- 0025 guarded the onboarding queue with UNIQUE (source, board): one company board can be
-- contributed at most once, which dedups a second vacancy on the same board and makes the
-- concurrent-duplicate race safe. That constraint spans every status, including 'rejected' —
-- so a board turned down as dead (its listing returned nothing the day it was checked) could
-- never be contributed again, even after the employer resumed posting. The contribution flow
-- documents the opposite ("a rejected real board can be re-contributed"), and the reject path
-- deliberately keeps the submitter's reward for exactly that reason: the link was real.
--
-- Narrowing the uniqueness to the live statuses keeps every guarantee that matters — 'pending',
-- 'review' and 'onboarded' still hold the identity, so a duplicate of a queued or already
-- onboarded board is still refused (ErrBoardAlreadyContributed, 409) and the reward is still
-- paid once per board — while a rejected row stops blocking anything. Re-contributing then
-- opens a NEW pending row, which is what the onboarding queue should see.
--
-- Applied to a fresh volume by initdb after 0048; on an existing prod volume this must be run
-- manually BEFORE deploying code that relies on it.

ALTER TABLE public.link_contributions
    DROP CONSTRAINT link_contributions_source_board_key;

CREATE UNIQUE INDEX link_contributions_source_board_active_key
    ON public.link_contributions USING btree (source, board)
    WHERE status <> 'rejected';
