-- Candidate-owned contacts (independent of resume_structured) plus last extract
-- attempt status for the current résumé upload. Contacts survive résumé delete;
-- extract status does not.
--
-- Expansive / nullable: safe to apply ahead of the binary that reads these columns.

ALTER TABLE users
    ADD COLUMN candidate_contacts jsonb,
    ADD COLUMN resume_extract_status text,
    ADD COLUMN resume_extract_detail text,
    ADD COLUMN resume_extract_for timestamp with time zone;

-- Backfill owned contacts from any stored structure (current or superseded).
UPDATE users
SET candidate_contacts = jsonb_strip_nulls(jsonb_build_object(
    'full_name', NULLIF(TRIM(COALESCE(resume_structured->>'full_name', '')), ''),
    'email', NULLIF(TRIM(COALESCE(resume_structured->>'email', '')), ''),
    'phone', NULLIF(TRIM(COALESCE(resume_structured->>'phone', '')), ''),
    'location', NULLIF(TRIM(COALESCE(resume_structured->>'location', '')), ''),
    'links', CASE
        WHEN jsonb_typeof(resume_structured->'links') = 'array'
             AND jsonb_array_length(resume_structured->'links') > 0
            THEN resume_structured->'links'
        ELSE NULL
    END
))
WHERE resume_structured IS NOT NULL
  AND candidate_contacts IS NULL
  AND (
      COALESCE(TRIM(resume_structured->>'full_name'), '') <> ''
      OR COALESCE(TRIM(resume_structured->>'email'), '') <> ''
      OR COALESCE(TRIM(resume_structured->>'phone'), '') <> ''
      OR COALESCE(TRIM(resume_structured->>'location'), '') <> ''
      OR (jsonb_typeof(resume_structured->'links') = 'array'
          AND jsonb_array_length(resume_structured->'links') > 0)
  );

-- Status from existing stamps: matching → ok; résumé present otherwise → pending.
UPDATE users
SET resume_extract_status = 'ok',
    resume_extract_for = resume_uploaded_at,
    resume_extract_detail = NULL
WHERE resume_uploaded_at IS NOT NULL
  AND resume_structured_uploaded_at IS NOT DISTINCT FROM resume_uploaded_at
  AND resume_structured IS NOT NULL;

UPDATE users
SET resume_extract_status = 'pending',
    resume_extract_for = resume_uploaded_at,
    resume_extract_detail = NULL
WHERE resume_uploaded_at IS NOT NULL
  AND resume_extract_status IS NULL;
