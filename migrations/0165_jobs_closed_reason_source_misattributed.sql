-- migrate: no-transaction
--
-- Adds 'source_misattributed' to jobs.closed_reason's allowed values (see 0071, and 0147 and
-- 0159 for the same widening one value each time): a posting closed not because it went away,
-- but because what we stored about it was wrong in a way that cannot be repaired in place.
--
-- The case that forced it, measured 2026-09-15: apploi's upstream API stopped honouring its
-- `employer` parameter, so every one of its 5833 boards fetched the same GLOBAL catalogue and
-- the adapter attributed each posting to whichever board's crawl happened to fetch it
-- (`Company: firstNonEmpty(e.Company, j.BrandName)` — and all 5833 board rows carry a company,
-- so the real brand_name was never once stored). 1,565,701 rows stand for 3,024 real jobs —
-- 518 copies each, under 518 different and mostly wrong employers. 264,022 of them reach live
-- search, where one title repeats down the page under a different company each time.
--
-- It needs its own label rather than reusing one of the seven that exist, because each of those
-- asserts something this close cannot:
--   'unseen'            — a crawl PROVED coverage and this posting was not in it. Here the
--                         crawls never proved anything; they fetched somebody else's postings.
--   'feed_removed'      — the source reported this posting taken down. Nothing was reported;
--                         most of these postings are still live on apploi today.
--   'expired' /
--   'probe_expired'     — both say the POSTING is over. These are not over; our copy of them
--                         is wrong, and closing them is a statement about us, not about them.
--   'board_unreachable' /
--   'feed_empty'        — both point at a board that stopped answering. Every one of these
--                         boards answered, every time, with a 200 and a full page.
--   'moderated'         — a moderator judged one posting. This is a mechanical consequence of
--                         a source defect, applied to a million rows at once.
-- Conflating them would cost the one thing a closed_reason is for: telling an operator which
-- mechanism fired, and therefore where to look. It is also the rollback: this label is the only
-- thing that can distinguish these rows from the ordinary closed ones afterwards.
--
-- Same large-table remedy as 0071, 0147 and 0159: DROP + re-ADD ... NOT VALID skips the row scan
-- under ACCESS EXCLUSIVE, and the follow-up VALIDATE CONSTRAINT takes only SHARE UPDATE
-- EXCLUSIVE, which blocks neither readers nor writers. Validation cannot fail: every existing
-- row already satisfies the widened list, since it is the old list plus one value.
ALTER TABLE public.jobs
    DROP CONSTRAINT IF EXISTS jobs_closed_reason_check;

ALTER TABLE public.jobs
    ADD CONSTRAINT jobs_closed_reason_check CHECK (closed_reason = ANY (ARRAY[
        ''::text,                     -- closed before the column existed; unknown
        'unseen'::text,               -- dropped off a board we crawl (CloseUnseenJobs / …BySource)
        'feed_removed'::text,         -- a streaming source reported it taken down
        'moderated'::text,            -- a moderator closed it
        'probe_expired'::text,        -- two consecutive expired liveness probes
        'expired'::text,              -- age rule: no close signal exists for this source
        'board_unreachable'::text,    -- chronic-board safety net: no successful crawl in 60+ days
        'feed_empty'::text,           -- empty-feed safety net: crawls succeed, feed carries nothing
        'source_misattributed'::text  -- what we stored about it is wrong and unrepairable in place
    ])) NOT VALID;

ALTER TABLE public.jobs
    VALIDATE CONSTRAINT jobs_closed_reason_check;
