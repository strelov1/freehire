-- Retry queue for the facetd mirror. cmd/search-drain writes each applied wave to facetd
-- right after Meilisearch accepts it; when that write fails, the wave's job ids land here,
-- and a later pass of the same worker, only while facetd is ready, re-derives each job's
-- CURRENT state from jobs (upsert / close / duplicate / delete) and sends it in one batch.
--
-- Reference-only like search_outbox (job_id + lease/retry bookkeeping, no payload): the
-- retry reads the row as it stands, so it is idempotent and immune to ordering, and the
-- table is bounded by the catalogue however long facetd stays down.
--
-- NO FOREIGN KEY to jobs, like search_delete_outbox: a hard-deleted job still has to be
-- removed from facetd, and its queue entry must outlive the row.

CREATE TABLE public.facetd_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- One live entry per job: the enqueue dedup key.
    job_id bigint NOT NULL UNIQUE,
    attempts bigint DEFAULT 0 NOT NULL,
    claimed_at timestamp with time zone,
    failed_at timestamp with time zone,
    last_error text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

CREATE INDEX facetd_outbox_claimable_idx ON public.facetd_outbox USING btree (id) WHERE (failed_at IS NULL);
