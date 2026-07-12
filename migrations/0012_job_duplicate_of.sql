-- duplicate_of marks a job as a non-canonical repost of another job in the same role
-- cluster ((company_slug, role_fingerprint), see 0003). Exactly one open row per
-- cluster is canonical (duplicate_of IS NULL) — the deterministic min(id) among the
-- cluster's open rows; the rest reference it. This collapses the catalogue to one card
-- per role WITHOUT deleting rows, so the job-reality repost/mass-posting counts (which
-- count the rows) are unaffected. The list, search index, and enrichment enqueue
-- exclude rows with a non-null duplicate_of; detail-by-slug still serves them.
--
-- Recomputed by RecomputeRoleDuplicates (reindex + recount cadence), so a closed canon
-- fails over to the next min(id). It is read/enqueue-time collapse, distinct from the
-- pre-insert dedup key (source, external_id).
--
-- Applied to a fresh volume by initdb after 0011; on an existing prod volume this ALTER
-- must be run manually BEFORE deploying code that reads the column, followed by a
-- RecomputeRoleDuplicates pass so existing clusters collapse.
ALTER TABLE public.jobs
    ADD COLUMN duplicate_of bigint REFERENCES public.jobs(id);

-- Partial index over the reposts (the minority): supports the recompute's cluster
-- writes and any duplicate_of lookups without bloating the canonical-heavy table.
CREATE INDEX jobs_duplicate_of_idx
    ON public.jobs USING btree (duplicate_of)
    WHERE duplicate_of IS NOT NULL;
