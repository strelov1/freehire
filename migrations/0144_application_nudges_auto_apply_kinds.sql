-- Widen application_nudges.kind to add the three auto-apply outcome nudges:
-- auto_apply_submitted (a queued attempt actually submitted), auto_apply_blocked
-- (permanently parked on a required question), auto_apply_failed (dead-lettered
-- after retries). See the add-auto-apply-outcome-notifications change.

ALTER TABLE public.application_nudges DROP CONSTRAINT application_nudges_kind_check;

ALTER TABLE public.application_nudges
    -- squawk-ignore constraint-missing-not-valid -- application_nudges is a one-shot-nudge ledger, nowhere near jobs-table scale, and migration 0084 widened this same CHECK the same way without NOT VALID, in production, without issue.
    ADD CONSTRAINT application_nudges_kind_check
    CHECK (kind IN ('follow_up', 'interview_prep', 'job_closed',
                     'auto_apply_submitted', 'auto_apply_blocked', 'auto_apply_failed'));
