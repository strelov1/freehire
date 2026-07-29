-- Ghost job signal: the two pieces of state a hedged "is this posting real" verdict
-- needs, which nothing else in the schema records.
--
-- Everything else the verdict reads already exists — the posting's own history
-- (jobreality), applications (user_jobs) and their linked mail (emails). The
-- verdict itself is NOT stored: it is time-dependent, so a stored class would go
-- stale sitting still, the same reason jobs.reality is computed rather than kept.
--
-- Applied to a fresh volume by initdb after 0052; on an existing prod volume run
-- this manually (SET ROLE hire) BEFORE deploying the code. The generated SELECTs
-- read jobs.*, so an unapplied column 42703s EVERY job read, not just this feature.

-- One person's statement that they applied to a posting and were never answered.
--
-- Deliberately NOT job_reports. That queue exists so a moderator can CLOSE a job,
-- and a report that is merely evidence cannot be expressed as a close: it needs to
-- accumulate, to be counted alongside other people's, and to be withdrawn when the
-- employer finally answers. Overloading one table with a moderation verdict and an
-- evidence tally would make both harder to reason about.
CREATE TABLE IF NOT EXISTS ghost_reports (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- One report per person per job; the composite uniqueness is the abuse bound,
    -- not a check in the service. The FK cascades purge on user or job delete.
    user_id     BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id      BIGINT      NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    -- The date the reporter states they applied. A claim is not evidence until this
    -- has aged past the `applied` silence threshold — the same bar a tracked
    -- application clears before the board calls it silent.
    applied_on  DATE        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Retraction rather than deletion: withdrawing is how the signal self-heals when
    -- an employer answers late, and keeping the row preserves the uniqueness bound so
    -- a retraction cannot be used to file repeatedly.
    retracted_at TIMESTAMPTZ,
    UNIQUE (user_id, job_id)
);

-- The evidence lookup reads by job, for a page of jobs at a time.
CREATE INDEX IF NOT EXISTS ghost_reports_job_idx
    ON ghost_reports (job_id) WHERE retracted_at IS NULL;

-- When the cross-check last found this posting's role absent from the company's own
-- crawled board. NULL means no evidence — which covers both "checked and present"
-- and "never checked", because the verdict cannot tell them apart and must not: a
-- board we do not crawl proves nothing about the employer.
--
-- The worker re-stamps every run, so the reader ignores a stamp older than its
-- freshness window. That way a worker that has stopped falls silent instead of
-- accusing the catalogue indefinitely from a frozen snapshot.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS ats_absent_at TIMESTAMPTZ;
