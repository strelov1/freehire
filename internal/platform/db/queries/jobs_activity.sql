-- name: ObserveJobActivity :execrows
-- The crawl's observation of one posting, written to job_activity: the SECOND command of every
-- full write (job.Upsert) and of the full TouchJob, in the transaction that wrote the jobs row.
-- jobs first, activity second is the global lock order, and a separate command (not a CTE of the
-- write) is what makes the row visible: the insert trigger creates the activity row AFTER the
-- insert statement's snapshot, so a same-statement CTE cannot see it.
--
-- last_seen_at never moves backwards (GREATEST with the stored value: a release-A writer or a
-- clock skew between hosts must not age a row that was just seen). hydrated_at advances only when
-- the write READ a body (body_read); a NULL stays NULL otherwise, which is how a raw, manual or
-- private row keeps "never checked". liveness_strikes is cleared only when the caller says so
-- (reset_strikes): a reopen is reset by the state trigger, so this is for the one case the
-- trigger cannot see, a full touch of an expired posting that stays closed. state_version is
-- never touched: an observation must not fence another observation.
--
-- :execrows because exactly one row is the invariant (the insert trigger gives every job its
-- row); the caller treats any other count as an error instead of silently dropping the stamp.
UPDATE job_activity
SET last_seen_at     = GREATEST(now(), last_seen_at),
    hydrated_at      = CASE WHEN sqlc.arg(body_read)::boolean THEN GREATEST(now(), hydrated_at) ELSE hydrated_at END,
    liveness_strikes = CASE WHEN sqlc.arg(reset_strikes)::boolean THEN 0 ELSE liveness_strikes END
WHERE job_id = sqlc.arg(job_id);
