-- Job reports: a moderation staging queue for user complaints about a live vacancy. A
-- signed-in user flags a problem job (stale, spam, fraud, …); the report lives here as
-- 'pending' and never changes the job on its own. A moderator then resolves it (optionally
-- soft-closing the job) or dismisses it. Like job_submissions, keeping reports out of the
-- canonical jobs table means no public read surface needs a new filter.

CREATE TABLE job_reports (
    id            BIGSERIAL PRIMARY KEY,

    -- The reporter. ON DELETE CASCADE: a report is owned by its author (the
    -- user_jobs/api_keys/submission ownership convention), so a deleted account takes its
    -- pending reports with it. The reported job is independent and survives.
    reported_by   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,

    -- The reported vacancy. ON DELETE CASCADE: a report is meaningless without its job, and
    -- the job is the thing being judged. Stored as the internal id (not the slug) so the
    -- report survives a reslug, like user_jobs.
    job_id        BIGINT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,

    -- Report content. reason is a closed vocabulary (the enrichment-enum convention),
    -- mirrored by the SPA; the service re-validates it for a clean 400. details is the
    -- required free-text explanation; contact_telegram is an optional way to reach the
    -- reporter.
    reason            TEXT NOT NULL
                          CHECK (reason IN ('no_response', 'not_relevant', 'spam', 'fraud', 'other')),
    details           TEXT NOT NULL,
    contact_telegram  TEXT NOT NULL DEFAULT '',

    -- Review lifecycle. status is a closed vocabulary. review_reason carries an optional
    -- dismissal note. reviewed_by/reviewed_at record the deciding moderator; ON DELETE SET
    -- NULL keeps the audit when a moderator is removed.
    status        TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'resolved', 'dismissed')),
    review_reason TEXT NOT NULL DEFAULT '',
    reviewed_by   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at   TIMESTAMPTZ,

    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- At most one open report per (user, job): while a user's report of a job is awaiting
-- review, a second report of the same job by that user is rejected (the repository maps the
-- unique violation to a 409). A different user reporting the same job is always allowed —
-- that overlap is the signal. A decided report no longer blocks re-reporting.
CREATE UNIQUE INDEX job_reports_open_user_job_key
    ON job_reports (reported_by, job_id) WHERE status = 'pending';

-- The moderator queue reads pending reports newest-first.
CREATE INDEX job_reports_pending_idx
    ON job_reports (created_at DESC) WHERE status = 'pending';
