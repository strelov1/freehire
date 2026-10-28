-- name: GetMarkerPassCursor :one
-- The last company a per-company marker pass finished (see migration 0211). No row is the
-- same as '' — start from the top — and the caller treats pgx.ErrNoRows that way.
SELECT after_company
FROM marker_pass_cursors
WHERE pass = sqlc.arg(pass);

-- name: SetMarkerPassCursor :exec
-- Record (or, with '', clear) a pass's resume point. Written as the pass goes, not only at
-- the end, because the run is routinely ended by its unit's timeout.
INSERT INTO marker_pass_cursors (pass, after_company, updated_at)
VALUES (sqlc.arg(pass), sqlc.arg(after_company), now())
ON CONFLICT (pass) DO UPDATE
SET after_company = EXCLUDED.after_company,
    updated_at    = now();
