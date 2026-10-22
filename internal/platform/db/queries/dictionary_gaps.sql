-- name: ListDictionaryGapsByKey :many
SELECT id, kind, raw_term, normalized_term, request_count, first_requested_at, last_requested_at, submitted_by, status, resolution_note
FROM dictionary_gaps
WHERE kind = $1 AND normalized_term = $2;

-- name: InsertDictionaryGap :one
INSERT INTO dictionary_gaps (kind, raw_term, normalized_term, submitted_by)
VALUES ($1, $2, $3, $4)
RETURNING request_count, status;

-- name: IncrementDictionaryGap :one
UPDATE dictionary_gaps
SET request_count = request_count + 1,
    last_requested_at = now()
WHERE kind = $1 AND normalized_term = $2 AND status = $3
RETURNING request_count, status;

-- name: ClaimDictionaryGapWrite :one
INSERT INTO dictionary_gap_daily (user_id, day, writes)
VALUES ($1, $2, 1)
ON CONFLICT (user_id, day) DO UPDATE
SET writes = dictionary_gap_daily.writes + 1
WHERE dictionary_gap_daily.writes < 20
RETURNING writes;
