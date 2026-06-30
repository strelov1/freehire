-- Fingerprint of a job's indexed (search-document) fields, written by the ingest
-- upsert (see internal/jobhash). It is the incremental-index change signal: a
-- re-ingest whose hash matches the stored one did not change searchable content
-- and needs no re-push to the live search index. Nullable with no default and no
-- backfill — a row's hash is populated on its next upsert; a NULL hash is DISTINCT
-- FROM any value, so a legacy row reports "changed" once and self-heals.
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS content_hash TEXT;
