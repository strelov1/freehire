-- The index the ghost cross-check's keyset scan needs.
--
-- Without it, paging through one aggregator's open postings in id order costs a scan of
-- that source's whole set plus a sort. Measured on prod before this index: the earlier
-- multi-source form of the query took 28 SECONDS per 2000-row page, because the planner
-- answered `source = ANY(...) ORDER BY id LIMIT n` with a Bitmap Heap Scan and a Sort
-- rather than an ordered index walk. Narrowing the query to one source is half the fix;
-- this index is the other half.
--
-- Partial on `closed_at IS NULL` because the worker only ever considers open postings,
-- which keeps the index proportional to the live catalogue rather than to its history.
--
-- Applied to a fresh volume by initdb after 0055. On an existing prod volume build it
-- CONCURRENTLY by hand, under nohup or screen: a CONCURRENTLY build dies with its ssh
-- session and leaves an INVALID index behind.
CREATE INDEX IF NOT EXISTS jobs_source_id_open_idx
    ON public.jobs (source, id) WHERE closed_at IS NULL;
