-- Synthetic enrichment facets (see specs/2026-06-17-synthetic-enrich-b-design):
-- four fields the LLM also emits but that are derived deterministically at ingest by
-- internal/lang + internal/jobfacts (via internal/jobderive). Like the other
-- dictionary facets (geography/skills/classification) these are SOURCE facts stored
-- beside — not inside — the `enrichment` JSONB, so the LLM enrichment worker never
-- clobbers them; jobview serves them dict-only (the deterministic value always wins,
-- the LLM's stays raw in the blob as a discovery signal).
--
-- posting_language: ISO 639-1 (e.g. en/pt/ru), "" when undetected.
-- employment_type:  enum enrich.EmploymentTypeValues, "" when unstated.
-- education_level:  enum enrich.EducationLevelValues, "" when unstated.
-- experience_years_min: minimum required years, NULL when unstated (0 is a valid
--   value — entry level — so NULL, not 0, is the "unknown" sentinel).
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS posting_language     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS employment_type      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS education_level      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS experience_years_min INTEGER;
