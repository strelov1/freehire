-- Apply before deploying readers of the source-stated application deadline.
-- A date is inclusive: applications remain open through this calendar day.
-- NULL means the source did not state a deadline; never infer one from posting age.
ALTER TABLE public.jobs ADD COLUMN IF NOT EXISTS application_deadline date;

COMMENT ON COLUMN public.jobs.application_deadline IS
    'Source-stated last application day, inclusive; NULL when unstated.';
