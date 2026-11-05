-- name: RecordDictionaryGap :one
-- One live (pending or accepted) row per term: the first ask inserts it and keeps its
-- raw_term and submitted_by, every later ask only bumps the count. A rejected row is
-- not live, so asking again opens a new pending one beside it. One upsert rather than
-- read-then-write, so two first asks racing both count instead of the second failing
-- on the partial unique index.
INSERT INTO dictionary_gaps (kind, raw_term, normalized_term, submitted_by)
VALUES ($1, $2, $3, $4)
ON CONFLICT (kind, normalized_term) WHERE status IN ('pending', 'accepted')
DO UPDATE SET request_count = dictionary_gaps.request_count + 1,
              last_requested_at = now()
RETURNING request_count, status;
