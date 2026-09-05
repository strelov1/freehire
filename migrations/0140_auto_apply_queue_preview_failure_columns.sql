-- Gives the preview pass its own transient-failure budget, separate from the real
-- submission pass's attempts/failed_at (migration 0116). Both passes used to share those
-- two columns (RecordAutoApplyFailure), which meant a transient preview-resolution error
-- (a flaky schema fetch, a browser launch hiccup) spent down the SAME retry budget the
-- real ATS submission depends on, and could dead-letter a row (failed_at set) before a
-- submission was ever attempted — reported to the candidate as "could not submit after
-- retrying," which never happened. See openspec/changes/auto-apply-review-tracking's own
-- review findings.
ALTER TABLE public.auto_apply_queue
    -- squawk-ignore prefer-bigint-over-int
    ADD COLUMN preview_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN preview_failed_at timestamptz;
