-- Company feedback moderation: a status lever a moderator can flip, and a
-- lightweight report table letting any signed-in user flag a specific review.
-- This is evidence for a moderator to act on, not a ticket queue — no
-- resolve/dismiss state machine, no reporter notification (internal/report's
-- job-report queue is the fuller version of this pattern, built for a much
-- higher-volume surface than one-review-per-user-per-company).
--
-- List/Count/Recount all filter to status = 'visible', so hiding a review drops
-- it out of the public list and out of the company's materialized average in
-- the same step.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: initdb runs migrations only on first
-- volume init, so on a persistent volume this does not auto-apply. Run before
-- deploying code that reads/writes these columns (same as 0088 before it).

ALTER TABLE public.company_feedback
    ADD COLUMN status text NOT NULL DEFAULT 'visible',
    ADD CONSTRAINT company_feedback_status_check CHECK (status = ANY (ARRAY['visible', 'hidden']::text[]));

CREATE TABLE public.company_feedback_reports (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feedback_id bigint NOT NULL REFERENCES public.company_feedback (id) ON DELETE CASCADE,
    reporter_user_id bigint REFERENCES public.users (id) ON DELETE SET NULL,
    reason text NOT NULL,
    created_at timestamp with time zone NOT NULL DEFAULT now(),
    CONSTRAINT company_feedback_reports_reason_check CHECK (reason = ANY (ARRAY[
        'spam', 'offensive', 'false_information', 'other'
    ]::text[]))
);

-- A second report by the same user on the same review is a no-op, not a duplicate row.
CREATE UNIQUE INDEX company_feedback_reports_reporter_feedback_uniq_idx
    ON public.company_feedback_reports (feedback_id, reporter_user_id) WHERE reporter_user_id IS NOT NULL;

-- The moderator view's read: every report, joined back to its feedback row.
CREATE INDEX company_feedback_reports_feedback_id_idx
    ON public.company_feedback_reports (feedback_id);

-- The reporter_user_id FK needs its own leading-column index (the uniq index
-- above has reporter_user_id second) — required so deleting an account isn't a
-- full-table scan; internal/db's TestEveryUserForeignKeyIsIndexed enforces
-- this for every FK into users.
CREATE INDEX company_feedback_reports_reporter_user_id_idx
    ON public.company_feedback_reports (reporter_user_id);

-- The per-user rate-limit check (company_feedback rows created recently by one user).
CREATE INDEX company_feedback_user_id_created_at_idx
    ON public.company_feedback (user_id, created_at) WHERE user_id IS NOT NULL;
