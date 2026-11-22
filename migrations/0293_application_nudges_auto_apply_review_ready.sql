-- Widen application_nudges.kind with auto_apply_review_ready: a tailored auto-apply
-- entry waits for the candidate's approval. Until now that was announced only in the
-- in-app notification center, which most candidates never open, so entries sat
-- unreviewed for weeks.

ALTER TABLE public.application_nudges DROP CONSTRAINT IF EXISTS application_nudges_kind_check;

ALTER TABLE public.application_nudges
    -- squawk-ignore constraint-missing-not-valid -- same small one-shot ledger that 0084 and 0144 widened the same way.
    ADD CONSTRAINT application_nudges_kind_check
    CHECK (kind IN ('follow_up', 'interview_prep', 'job_closed',
                     'auto_apply_submitted', 'auto_apply_blocked', 'auto_apply_failed',
                     'auto_apply_review_ready'));
