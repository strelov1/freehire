-- english_level dictionary facet: the required English level, derived
-- deterministically at ingest by internal/jobfacts (via internal/jobderive) from the
-- description text. Like the other synthetic facets (migration 0023) it is a SOURCE
-- fact stored beside — not inside — the `enrichment` JSONB, so the LLM enrichment
-- worker never clobbers it; jobview serves it dict-only (the deterministic value
-- always wins, the LLM's stays raw in the blob as a discovery signal).
--
-- english_level: enum enrich.EnglishLevelValues (none/a1/a2/b1/b2/c1/c2/native),
--   "" when unstated.
ALTER TABLE jobs
    ADD COLUMN IF NOT EXISTS english_level TEXT NOT NULL DEFAULT '';
