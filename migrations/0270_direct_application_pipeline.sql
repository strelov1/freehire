-- The employer's side of an on-site application: where it stands in the employer's hiring
-- pipeline, and the CV exactly as the employer received it. See
-- openspec/changes/add-employer-applicant-pipeline.
--
-- stage is the employer's own pipeline position. Every application starts at 'new', which
-- describes every row written before this column truthfully: no employer could act on one,
-- because nothing but the email showed it to them. stage_changed_at is NULL until the
-- employer first moves it.
--
-- The CHECK is added and validated in this one statement, as in 0190: the NOT VALID /
-- VALIDATE split avoids holding a lock while a big table is scanned, and this table holds a
-- few hundred rows at most.
-- squawk-ignore constraint-missing-not-valid
ALTER TABLE public.direct_applications
    ADD COLUMN IF NOT EXISTS stage text NOT NULL DEFAULT 'new'
        CONSTRAINT direct_applications_stage_check
            CHECK (stage IN ('new', 'reviewing', 'interview', 'offer', 'hired', 'rejected')),
    ADD COLUMN IF NOT EXISTS stage_changed_at timestamptz;

-- direct_application_cvs: the CV file sent with an application, kept so the employer can
-- open it from their inbox later. A copy rather than a reference, like the answers snapshot:
-- a tailored CV the candidate edits or deletes afterwards must not change what the employer
-- was sent. One row per application, replaced when an unsent application is retried.
--
-- In Postgres rather than the S3 blob store: the row then lives and dies with its
-- application (CASCADE through direct_applications, which cascades from users and jobs), so
-- account deletion needs no second cleanup path, and the volume is one CV per application to
-- an on-site vacancy, capped per candidate per day. Applications written before this table
-- have no copy; their CV reached the employer by email only.
CREATE TABLE IF NOT EXISTS public.direct_application_cvs (
    application_id bigint PRIMARY KEY REFERENCES public.direct_applications (id) ON DELETE CASCADE,
    filename       text   NOT NULL,
    content_type   text   NOT NULL,
    content        bytea  NOT NULL
);
