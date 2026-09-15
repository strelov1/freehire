-- squawk-ignore-file constraint-missing-not-valid -- a PRIMARY KEY has no NOT VALID form, so the rule cannot be satisfied; the per-statement form does not suppress it either, because the two rules this ALTER trips need two `-- squawk-ignore` lines and only the last one counts as "immediately before". See the argument at the ADD CONSTRAINT below.
-- squawk-ignore-file adding-serial-primary-key-field -- same statement, same reason: job_search_pings is three days old and holds a few hundred rows, so the index build behind the key is milliseconds. File-level for the same "only the last line counts" reason.
-- A posting is worth announcing twice: once when it appears, and once when it closes.
--
-- The second announcement is NOT a deletion. A closed posting's page stays at HTTP 200
-- and keeps its JobPosting markup with validThrough moved into the past, which is one of
-- the three ways Google documents for retiring a job posting. So what a closure needs is
-- a re-crawl — the same URL_UPDATED a new posting gets — and sending URL_DELETED instead
-- would be a misuse of the API against a page that is still online, which is the kind of
-- thing the quota is withdrawn for.
--
-- kind splits the ledger by EVENT, so "announced when it appeared" and "announced when it
-- closed" are separate facts about the same posting rather than one that overwrites the
-- other. Without it, a closure could never be announced: the anti-join in ListJobsToPing
-- would find the row from the posting's creation and skip it forever.
--
-- 'created' as the default and the backfill value because that is what every existing row
-- is: migration 0162 shipped with only the one event.
ALTER TABLE public.job_search_pings
    ADD COLUMN kind text NOT NULL DEFAULT 'created';

-- The primary key has to widen with it — one posting may now hold two rows per engine,
-- and it is the PK that both makes the send idempotent under ON CONFLICT DO NOTHING and
-- serves the anti-join. Safe to do in one statement here and only here: the table is days
-- old and holds a few hundred rows, so the rewrite is instant. It would not be on jobs.
ALTER TABLE public.job_search_pings
    DROP CONSTRAINT job_search_pings_pkey;

-- Both rules below are about the ACCESS EXCLUSIVE lock a primary key takes while its
-- index builds. job_search_pings was created three days ago by migration 0162 and holds a
-- few hundred rows, so that build is instant; the same statement against jobs (11M rows)
-- would be the outage these rules exist to prevent. NOT VALID is not available for a
-- primary key, and doing this concurrently would mean dropping the old key, living without
-- one and adding the new — a window in which the ON CONFLICT that makes every send
-- idempotent has nothing to conflict on.
ALTER TABLE public.job_search_pings
    ADD CONSTRAINT job_search_pings_pkey PRIMARY KEY (job_id, engine, kind);

-- The reporting index widens the same way: "what has this engine been told, and when"
-- is now a question per event, since the two are spent from the same daily budget and
-- the whole point of separating them is to see which one is consuming it.
-- squawk-ignore require-concurrent-index-deletion -- a few hundred rows, and a concurrent drop cannot run inside this file's transaction
DROP INDEX IF EXISTS job_search_pings_engine_pinged_at_idx;

-- squawk-ignore require-concurrent-index-creation -- a few hundred rows; CONCURRENTLY would force this whole file out of its transaction, and the primary key swap above would stop being atomic with it
CREATE INDEX job_search_pings_engine_kind_pinged_at_idx
    ON public.job_search_pings (engine, kind, pinged_at DESC);
