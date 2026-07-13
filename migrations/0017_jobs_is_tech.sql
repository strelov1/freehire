-- is_tech is the tri-state deterministic technical/non-technical signal derived
-- from a job's title and category (internal/jobderive): TRUE for a recognized
-- technical category, FALSE for a known non-technical category or a confident
-- non-tech title, and NULL when neither resolves (unknown). Nullable on purpose —
-- NULL is the honest "unclassified" state, kept measurable rather than coerced.
-- Backfill existing rows with cmd/backfill-derive, then reindex.
ALTER TABLE jobs ADD COLUMN is_tech boolean;
