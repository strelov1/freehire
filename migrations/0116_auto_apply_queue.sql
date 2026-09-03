-- Queue for cmd/auto-apply: one row per (candidate, job) attempt to submit an application
-- without the candidate present.
--
-- Columns and lease semantics mirror apply_form_outbox: claimed_at is a lease so a dead
-- worker's rows return without a reaper process, attempts/failed_at bound the retry of a
-- transient failure, and last_error keeps the reason queryable.
--
-- One addition apply_form_outbox has no equivalent for: blocked_at. A capture either
-- succeeds or transiently fails and retries — there is no third outcome. A submission
-- attempt has one: the job's form asks a required question the candidate's profile cannot
-- answer. That is not a transient failure (retrying now would resolve nothing) and it is
-- not success, so it needs its own terminal marker, separate from failed_at, and a place to
-- record which questions stopped it (`unmapped`) so the gap is legible without replaying
-- the attempt.
--
-- What populates this queue (a candidate action vs. a standing per-user rule) is out of
-- scope for this table — see openspec/changes/auto-apply-worker/design.md. Rows arrive by
-- some other write path; cmd/auto-apply only claims and resolves them.
--
-- On a successful submission the row is deleted, the same way DeleteApplyFormEntry retires
-- a completed capture: jobtracking.MarkJobApplied is the durable record of "this candidate
-- applied to this job", so nothing here needs to survive success as well.
CREATE TABLE public.auto_apply_queue (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    job_id     bigint      NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    -- squawk-ignore prefer-bigint-over-int
    attempts   integer     NOT NULL DEFAULT 0, -- a small, bounded retry counter capped by
    -- AUTO_APPLY_MAX_ATTEMPTS, never remotely close to the 32-bit limit — same as every
    -- other outbox table's attempts column: semantic_outbox, search_outbox, search_delete_outbox
    claimed_at timestamptz,
    failed_at  timestamptz,
    blocked_at timestamptz,
    last_error text        NOT NULL DEFAULT '',
    -- [{id, label, required, reason}], set when blocked_at is. NULL otherwise — this is
    -- the one attempt's own findings, not a persisted form schema (deliberately not built
    -- here; see design.md's "DOM is scanned live" decision).
    unmapped   jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- One live attempt per (candidate, job) at a time — the enqueue dedup key, same role
    -- apply_form_outbox's UNIQUE (job_id) plays for a capture.
    UNIQUE (user_id, job_id)
);

-- Partial index over claimable (not dead-lettered, not parked) entries, mirroring
-- apply_form_outbox_claimable_idx.
CREATE INDEX auto_apply_queue_claimable_idx
    ON public.auto_apply_queue USING btree (id)
    WHERE (failed_at IS NULL AND blocked_at IS NULL);
