-- Widen application_nudges.kind to add job_closed: the third lifecycle nudge —
-- a listing the candidate is still actively tracking (an application in any
-- silence-accruing stage, per userjob.SilenceThresholdDays) closes. See the
-- centralize-lifecycle-notifications change (0082/0083) for the table itself.

ALTER TABLE public.application_nudges DROP CONSTRAINT application_nudges_kind_check;

ALTER TABLE public.application_nudges
    ADD CONSTRAINT application_nudges_kind_check
    CHECK (kind IN ('follow_up', 'interview_prep', 'job_closed'));
