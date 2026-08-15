-- Company feedback moves from one row per (user, company) to one row per
-- (user, company, feedback_type): a user may leave at most one review per
-- category (interview, culture, ...) on a given company, but may now leave a
-- second review on the same company under a different category instead of
-- being forced to overwrite their only one. Edit-by-resubmit still applies,
-- scoped to the (user, company, category) triple.
--
-- APPLY TO PROD MANUALLY BEFORE DEPLOY: the new binary's UpsertCompanyFeedback
-- names this migration's unique index as its ON CONFLICT target, which does
-- not exist until this runs — an unapplied index makes every feedback write
-- fail with 42P10 (same ordering requirement as 0088/0089).

DROP INDEX public.company_feedback_user_company_uniq_idx;

CREATE UNIQUE INDEX company_feedback_user_company_type_uniq_idx
    ON public.company_feedback (user_id, company_slug, feedback_type) WHERE user_id IS NOT NULL;
