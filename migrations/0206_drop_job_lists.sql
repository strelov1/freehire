-- Drop job_lists and job_list_items (0135/0136). Named job lists — the "+ New list"
-- popover beside Save, /my/lists and the public /l/:slug page — are retired in this
-- same change; "Save to board" (user_jobs.saved_at) is the one way to keep a job, and
-- it never read or wrote these tables.
--
-- 37 lists across 29 accounts, 97 items and 5 public links in prod on 2026-10-26,
-- archived on the host at /opt/freehire/backups/job_lists-20261026.sql
-- (column-inserts), because a DROP is the one step nothing else can undo. Public
-- /l/ links simply stop resolving.
--
-- Before the drop, every listed job becomes a SAVED job of the list's owner, so
-- nothing a person kept disappears from their account: it moves to the one place left
-- to keep a job. A job in several lists is one saved row, dated by its earliest add.
-- An already-saved job keeps its own saved_at; any other user_jobs row (a view, a
-- dismissal) only gains saved_at, exactly what SaveJob would write.
--
-- Items first: job_list_items references job_lists. CASCADE is deliberately NOT used:
-- if something still depends on either table, the drop should fail and say so rather
-- than quietly take the dependant with it.
INSERT INTO public.user_jobs (user_id, job_id, saved_at)
SELECT l.user_id, i.job_id, min(i.added_at)
  FROM public.job_list_items i
  JOIN public.job_lists l ON l.id = i.list_id
 GROUP BY l.user_id, i.job_id
ON CONFLICT (user_id, job_id)
    DO UPDATE SET saved_at = COALESCE(user_jobs.saved_at, EXCLUDED.saved_at);

-- The rule below fires on every DROP TABLE, because dropping one breaks existing
-- clients. Their only clients are the endpoints removed in this change, and their rows
-- are archived above.
-- squawk-ignore ban-drop-table
DROP TABLE IF EXISTS public.job_list_items;
-- squawk-ignore ban-drop-table
DROP TABLE IF EXISTS public.job_lists;
