-- Minimal schema for the job aggregator.
-- Applied automatically by Postgres on first volume init
-- (the folder is mounted into /docker-entrypoint-initdb.d) and also
-- serves as the schema source for sqlc.

CREATE TABLE IF NOT EXISTS jobs (
    id           BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source       TEXT        NOT NULL,
    external_id  TEXT        NOT NULL,
    url          TEXT        NOT NULL,
    title        TEXT        NOT NULL,
    company      TEXT        NOT NULL DEFAULT '',
    location     TEXT        NOT NULL DEFAULT '',
    remote       BOOLEAN     NOT NULL DEFAULT TRUE,
    description  TEXT        NOT NULL DEFAULT '',
    posted_at    TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- One job per source = one record (the basis for deduplication).
    UNIQUE (source, external_id)
);

-- Composite index for posted_at-ordered access and queries that filter/sort on posted_at
-- alone (leading column). NOTE: ListJobs now orders by created_at, served by the partial
-- jobs_open_created_at_id_idx in migration 0033 — not this index.
CREATE INDEX IF NOT EXISTS jobs_posted_at_id_idx ON jobs (posted_at DESC NULLS LAST, id DESC);
CREATE INDEX IF NOT EXISTS jobs_source_idx ON jobs (source);
