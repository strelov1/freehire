-- A user may store ONE résumé, kept in S3 object storage (not in Postgres). These
-- columns are the pointer: the object key and when it was uploaded. The blob itself
-- lives in the bucket under `resumes/<user_id>` (see internal/blobstore). Nullable:
-- a user with no stored résumé has both NULL — backward-compatible with existing rows.
--
-- Like every migration here it applies on fresh volume init and is the schema source
-- for sqlc; existing volumes/prod need a manual apply BEFORE rolling the new binary
-- (the open versioned-migration-runner seam).

ALTER TABLE users
    ADD COLUMN resume_object_key  TEXT,
    ADD COLUMN resume_uploaded_at TIMESTAMPTZ;

-- Rollback (inverse), if ever needed:
--   ALTER TABLE users DROP COLUMN resume_object_key, DROP COLUMN resume_uploaded_at;
