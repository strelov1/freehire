-- direct_applications: a candidate's application to an employer vacancy that takes
-- applications on freehire (job.IsDirectApply). See openspec/changes/add-in-app-apply.
--
-- The row is written BEFORE the email that delivers it, and sent_at only after the email
-- succeeded: the opposite order would let a failed send look sent and lose the application
-- silently. A row with sent_at NULL is therefore an application the employer has not
-- received, and the candidate applying again re-sends it rather than adding a second row —
-- UNIQUE (job_id, user_id) is what makes "one application per vacancy" structural.
--
-- sending_at is the claim a delivery holds while the email is in flight. Without it two
-- overlapping requests (a double click, a client retrying a slow response) would both find
-- the row unsent and both email the employer. A claim older than two minutes is treated as
-- abandoned by a request that died mid-send; a failed delivery releases it at once.
--
-- answers is a snapshot of each question's id, label and type beside the answer, so the
-- record still reads correctly after the employer edits the questions.
--
-- cv_id is SET NULL on delete: a candidate deleting a CV later does not un-send what the
-- employer already received, and the record of having applied stays.
CREATE TABLE IF NOT EXISTS public.direct_applications (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id     bigint      NOT NULL REFERENCES public.jobs (id) ON DELETE CASCADE,
    user_id    bigint      NOT NULL REFERENCES public.users (id) ON DELETE CASCADE,
    cv_id      uuid        REFERENCES public.cvs (id) ON DELETE SET NULL,
    note       text        NOT NULL DEFAULT '',
    answers    jsonb       NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    sending_at timestamptz,
    sent_at    timestamptz,
    CONSTRAINT direct_applications_job_user_key UNIQUE (job_id, user_id)
);

-- The daily cap counts one candidate's applications created since midnight UTC, and the
-- user_id lookups on account deletion need the same leading column.
CREATE INDEX IF NOT EXISTS direct_applications_user_created_idx
    ON public.direct_applications (user_id, created_at);

-- Deleting a CV sets cv_id NULL here; without an index that is a sequential scan per delete.
CREATE INDEX IF NOT EXISTS direct_applications_cv_idx
    ON public.direct_applications (cv_id) WHERE cv_id IS NOT NULL;
