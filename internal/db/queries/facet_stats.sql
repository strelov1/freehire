-- Facet-distribution snapshot (insights_facet_stats), recomputed by
-- cmd/rollup-facets as an atomic delete-and-reinsert, and the read query the public
-- /api/v1/stats/facets endpoint serves from it. The source is Meilisearch's facet
-- count (not `jobs`), so the worker inserts rows it already holds in memory rather
-- than a set-based INSERT ... SELECT.

-- name: DeleteAllFacetStats :exec
-- First half of the atomic rebuild: clear the snapshot. Run in the same transaction
-- as the InsertFacetStat loop so a reader never sees an empty or partial table.
DELETE FROM insights_facet_stats;

-- name: InsertFacetStat :exec
-- Second half of the atomic rebuild: one row per (facet, value). Called once per
-- value the worker computed, inside the same transaction as DeleteAllFacetStats.
INSERT INTO insights_facet_stats (facet, value, count)
VALUES (sqlc.arg('facet'), sqlc.arg('value'), sqlc.arg('count'));

-- name: ListFacetStats :many
-- The whole snapshot, ordered by facet then count DESC so the reader can take the
-- top-N per facet without re-sorting. Aggregate only — per-value counts, no
-- record-level data.
SELECT facet, value, count
FROM insights_facet_stats
ORDER BY facet, count DESC;
