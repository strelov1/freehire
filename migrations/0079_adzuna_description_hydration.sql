-- Full job descriptions for Adzuna postings.
--
-- Adzuna's search API hands back only a ~500-character snippet of a posting's body ("we
-- currently only provide a snippet of the job description" — developer.adzuna.com); the full
-- text lives only on Adzuna's own hosted job page, as an application/ld+json JobPosting block.
-- Not every stored URL reaches that page: Adzuna serves two shapes, its own hosted
-- "/details/..." page (200, scrapeable) and a "/land/ad/..." ad-network tracking redirect that
-- answers its own branded "Access Denied" page even to a browser-like request (confirmed live
-- 2026-08-08). Only the first is queued.
--
-- The fetched text overwrites jobs.description directly (via the existing UpdateJobDescription
-- query, which recomputes content_hash so the row re-indexes) — no new payload table, unlike
-- apply_forms, because the payload already has a home. What IS new is what apply_forms needed
-- two tables for: a transient queue and a permanent "already done" marker, because once the
-- queue entry is deleted after a successful capture nothing else would stop the very next crawl
-- of that same posting from queueing it again.
--
-- Applied to a fresh volume by initdb after 0078; on an existing prod volume run this manually
-- (SET ROLE hire) BEFORE deploying code that reads it. Purely additive, two new tables, no
-- column added to jobs — jobs is the hot table and a long-running read plus an ALTER on it takes
-- the site down.

-- The capture queue. Columns and lease semantics are apply_form_outbox's, copied rather than
-- redesigned: claimed_at is a lease so a dead worker's rows return without a reaper process, and
-- attempts/failed_at/last_error bound the retry and keep the reason queryable.
CREATE TABLE IF NOT EXISTS adzuna_description_outbox (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- One live entry per job — the enqueue dedup key.
    job_id     bigint      NOT NULL UNIQUE REFERENCES jobs (id) ON DELETE CASCADE,
    attempts   integer     NOT NULL DEFAULT 0,
    claimed_at timestamptz,
    failed_at  timestamptz,
    last_error text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Partial index over claimable (not dead-lettered) entries, mirroring
-- apply_form_outbox_claimable_idx.
CREATE INDEX IF NOT EXISTS adzuna_description_outbox_claimable_idx
    ON adzuna_description_outbox USING btree (id) WHERE (failed_at IS NULL);

-- The permanent marker: this job's description has already been hydrated, so the ingest write
-- path's enqueue must not queue it again. No payload here — that is jobs.description itself —
-- so this table carries nothing to read, only to check NOT EXISTS against, same role apply_forms
-- plays for its own outbox's gate.
CREATE TABLE IF NOT EXISTS adzuna_description_hydrated (
    job_id      bigint      PRIMARY KEY REFERENCES jobs (id) ON DELETE CASCADE,
    hydrated_at timestamptz NOT NULL DEFAULT now()
);
