-- Adds 'advanced_search' to the onboarding sequence's step vocabulary: a new
-- letter introducing the filter panel (role, region, skills, include/exclude,
-- saving a search) that now sits between the welcome greeting and the no-alert
-- nudge, which moves later to make room for it.

ALTER TABLE public.onboarding_emails
    DROP CONSTRAINT onboarding_emails_step_check,
    ADD CONSTRAINT onboarding_emails_step_check
        CHECK (step IN ('welcome', 'advanced_search', 'no_alert', 'open_source'));
