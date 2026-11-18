-- name: UpsertLinkedInPostingRead :exec
-- Record that a LinkedIn posting's detail was just read and was not on-site (see migration
-- 0283). Idempotent: a re-read moves read_at forward and may change the method.
INSERT INTO linkedin_postings_read (posting_id, apply_method, read_at)
VALUES (sqlc.arg(posting_id), sqlc.arg(apply_method), now())
ON CONFLICT (posting_id) DO UPDATE SET apply_method = EXCLUDED.apply_method, read_at = EXCLUDED.read_at;

-- name: LinkedInPostingsReadSince :many
-- Postings read and found non-native since the cutoff: the negative half of the LinkedIn
-- seen-set. An older row is not returned, so the posting is read again.
SELECT posting_id FROM linkedin_postings_read WHERE read_at >= sqlc.arg(read_since)::timestamptz;

-- name: PruneLinkedInPostingsRead :exec
-- Drop rows no seen-set can use any more; the caller passes a cutoff well past the window.
DELETE FROM linkedin_postings_read WHERE read_at < sqlc.arg(older_than)::timestamptz;
