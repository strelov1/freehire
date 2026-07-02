-- The résumé verdict is now a deterministic market-coverage computation over the
-- profile's skills and the selected role's live vacancy facets (internal/verdict):
-- how many open vacancies the profile's skills reach and which missing skill unlocks
-- the most new ones. The old LLM "coherence" layer is removed, so the derived-analysis
-- blob added in 0038 has no writer or reader left — drop the column.
--
-- IF EXISTS so a re-run (or a fresh volume where 0038's column co-exists in the same
-- init pass) is a no-op. Like every migration here it applies on fresh volume init and
-- is the schema source for sqlc; existing volumes/prod need a manual apply (the open
-- versioned-migration-runner seam from AGENT.md) — apply this BEFORE rolling the new
-- server binary, whose profile fetch query no longer selects the dropped column.

ALTER TABLE search_profiles
    DROP COLUMN IF EXISTS resume_analysis;
