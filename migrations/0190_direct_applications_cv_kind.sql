-- direct_applications.cv_kind: which CV an application sent — a CV the candidate tailored
-- for that vacancy ('tailored', cv_id names it) or the CV file they uploaded to their
-- profile ('resume', cv_id is NULL). See openspec/changes/add-in-app-apply.
--
-- Without it a NULL cv_id would mean two different things: "sent the uploaded file" and
-- "sent a tailored CV the candidate has since deleted" (cv_id is SET NULL on delete).
-- Every row written before this column existed sent a tailored or builder CV, so the
-- default describes them truthfully.
--
-- The CHECK is added and validated in this one statement. The NOT VALID / VALIDATE split
-- exists to avoid holding a lock while a big table is scanned; this table held a handful
-- of rows when the column landed (the feature shipped days earlier), so the scan is instant.
-- squawk-ignore constraint-missing-not-valid
ALTER TABLE public.direct_applications
    ADD COLUMN IF NOT EXISTS cv_kind text NOT NULL DEFAULT 'tailored'
        CONSTRAINT direct_applications_cv_kind_check CHECK (cv_kind IN ('tailored', 'resume'));
