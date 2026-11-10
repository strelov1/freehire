-- Skill co-occurrence snapshot (insights_skill_neighbours), recomputed by
-- cmd/rollup-facets as an atomic delete-and-reinsert, and the read the public
-- /api/v1/stats/skills/{slug}/neighbours endpoint serves from it.

-- name: DeleteAllSkillNeighbours :exec
-- First half of the atomic rebuild; run in the same transaction as the inserts.
DELETE FROM insights_skill_neighbours;

-- name: InsertSkillNeighbour :exec
INSERT INTO insights_skill_neighbours (skill, neighbour, count)
VALUES (sqlc.arg('skill'), sqlc.arg('neighbour'), sqlc.arg('count'));

-- name: ListSkillNeighbours :many
-- One skill's neighbours, most frequent first. Aggregate only.
SELECT neighbour, count
FROM insights_skill_neighbours
WHERE skill = sqlc.arg('skill')
ORDER BY count DESC, neighbour;
