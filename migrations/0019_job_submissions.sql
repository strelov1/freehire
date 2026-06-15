-- Public job submissions: a moderation staging queue. Any authenticated user can submit
-- a vacancy; it lives here as 'pending' and only becomes a real jobs row when a moderator
-- approves it. Keeping submissions out of the canonical jobs table means every public read
-- surface (list/search/company/sitemap/Meilisearch) needs no new "is it approved" filter.

CREATE TABLE job_submissions (
    id            BIGSERIAL PRIMARY KEY,

    -- The contributor. ON DELETE CASCADE: a submission is owned by its submitter (the
    -- user_jobs/api_keys ownership convention), so a deleted account takes its pending
    -- submissions with it. The minted job (once approved) is independent and survives.
    submitted_by  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- Submission content: the same shape a moderator create accepts. url is the dedup key
    -- and, on approval, the job's external_id; source defaults to '' (the approve path
    -- maps empty to 'manual', like UpsertManualJob).
    url           TEXT NOT NULL,
    source        TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL,
    company       TEXT NOT NULL,
    location      TEXT NOT NULL DEFAULT '',
    remote        BOOLEAN NOT NULL DEFAULT false,
    description   TEXT NOT NULL DEFAULT '',
    posted_at     TIMESTAMPTZ,

    -- Review lifecycle. status is a closed vocabulary (the enrichment-enum convention).
    -- review_reason carries an optional rejection note. reviewed_by/reviewed_at record the
    -- deciding moderator; ON DELETE SET NULL keeps the audit when a moderator is removed.
    -- job_id points at the vacancy minted on approval; ON DELETE SET NULL so deleting the
    -- job does not delete its submission history.
    status        TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'approved', 'rejected')),
    review_reason TEXT NOT NULL DEFAULT '',
    reviewed_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at   TIMESTAMPTZ,
    job_id        BIGINT REFERENCES jobs(id) ON DELETE SET NULL,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- At most one pending submission per URL: while a URL is awaiting review, a second
-- submission of it is rejected (the repository maps the unique violation to a 409). A
-- decided submission (approved/rejected) no longer blocks resubmission of that URL.
CREATE UNIQUE INDEX job_submissions_pending_url_key
    ON job_submissions (lower(url)) WHERE status = 'pending';

-- The moderator queue reads pending submissions newest-first.
CREATE INDEX job_submissions_pending_idx
    ON job_submissions (created_at DESC) WHERE status = 'pending';

-- "My submissions" reads one user's submissions newest-first.
CREATE INDEX job_submissions_by_user_idx
    ON job_submissions (submitted_by, created_at DESC);
