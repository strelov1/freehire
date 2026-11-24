-- Drop four columns that nothing reads or writes any more. Archive the rows on the host
-- before releasing (see the pull request), because a DROP COLUMN is the one step nothing
-- else can undo. CASCADE is deliberately NOT used: if something still depends on a column
-- here, the drop should fail and say so rather than quietly take the dependant with it.
--
-- experience_employments.period_start / period_end (0047, text) were the free-text dates.
-- 0135 replaced them with period_start_year/month and period_end_year/month, kept them only
-- so cmd/backfill-experience-dates had something to read from, and promised a follow-up
-- drop. That backfill ran and its command was deleted in e848a5dd3; since then no query
-- names the pair, and the index that ordered by period_start was replaced in 0135 too.
--
-- users.resume_embedding / resume_embedding_model (0002) fed the /my/recommendations feed.
-- #1905 removed that endpoint's reads and the SET that wrote them; the only statement left
-- is ClearUserResume nulling them, which goes in the same change as this file.
ALTER TABLE public.experience_employments
    -- squawk-ignore ban-drop-column -- no reader since the backfill in e848a5dd3; see above
    DROP COLUMN IF EXISTS period_start,
    -- squawk-ignore ban-drop-column -- same as period_start
    DROP COLUMN IF EXISTS period_end;

ALTER TABLE public.users
    -- squawk-ignore ban-drop-column -- no writer since #1905; see above
    DROP COLUMN IF EXISTS resume_embedding,
    -- squawk-ignore ban-drop-column -- same as resume_embedding
    DROP COLUMN IF EXISTS resume_embedding_model;
