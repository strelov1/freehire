-- Per-user saved searches: a named snapshot of the job-search filter state so a
-- signed-in user can re-apply a filter set in one click from any device. The
-- `query` column stores the canonical search query string — exactly what the web
-- filter layer serializes to the URL and what GET /api/v1/jobs/search reads — so a
-- saved search needs no bespoke filter format; an empty string is the valid
-- "show all" view. Names are unique per user (so the picker can list them) and
-- length-bounded as a backstop to the service-layer validation. Applied
-- automatically by Postgres on first volume init (same as 0001) and also serves as
-- schema source for sqlc. Existing volumes/prod need a manual apply (the
-- versioned-migration-runner seam from AGENT.md remains open).

CREATE TABLE IF NOT EXISTS saved_searches (
    id         BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- Display name, trimmed and bounded by the service; the CHECK is the backstop.
    name       TEXT        NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 100),
    -- Canonical search query string (the URL query the search API reads). May be
    -- empty, which represents the unfiltered "show all" view.
    query      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Distinct names per user so the "My filters" picker has stable labels.
    UNIQUE (user_id, name)
);

-- List-by-owner, most-recently-updated first (the picker order).
CREATE INDEX IF NOT EXISTS saved_searches_user_updated_idx
    ON saved_searches (user_id, updated_at DESC);
