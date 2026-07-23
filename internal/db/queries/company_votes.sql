-- Per-(user, company) thumbs votes. Unlike a job vote (a nullable column on the
-- user_jobs row that persists for other marks), a company_votes row exists solely to
-- hold the vote, so clearing a vote DELETEs the row. The domain layer branches in Go
-- (read current, then delete-if-same-else-upsert) and recomputes the company's
-- materialized counters in the same transaction.

-- name: LockCompanyForVote :exec
-- Take the company row's lock so concurrent votes on the same company serialize
-- (see LockJobForVote for the drift this prevents). Called first in the vote
-- transaction.
SELECT 1 FROM companies WHERE slug = $1 FOR UPDATE;

-- name: CompanySlugExists :one
-- Whether a company with this slug exists — the cheap existence check the vote
-- path uses to return 404 before touching company_votes (whose FK would otherwise
-- surface a bad slug as an opaque error on insert, or a silent no-op on clear).
SELECT EXISTS (SELECT 1 FROM companies WHERE slug = $1);

-- name: GetCompanyVote :one
-- The caller's current vote for a company (0 when none). Always returns one row.
SELECT COALESCE((SELECT vote FROM company_votes WHERE user_id = $1 AND company_slug = $2), 0)::smallint AS my_vote;

-- name: UpsertCompanyVote :exec
-- Set a user's company vote to $3 (-1 or 1), inserting or overwriting in place.
-- Toggle-to-clear is handled by DeleteCompanyVote, chosen by the domain layer.
INSERT INTO company_votes (user_id, company_slug, vote)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, company_slug) DO UPDATE
  SET vote = EXCLUDED.vote, updated_at = now();

-- name: DeleteCompanyVote :exec
-- Remove a user's company vote (toggle-clear or the DELETE endpoint). No-op when
-- absent.
DELETE FROM company_votes WHERE user_id = $1 AND company_slug = $2;

-- name: RecountCompanyVotes :one
-- Recompute a single company's materialized vote counters from company_votes and
-- return them. Run as its own statement AFTER the vote write within one transaction.
-- Scoped to one company_slug via company_votes_company_slug_idx.
UPDATE companies SET
    upvote_count   = (SELECT count(*) FROM company_votes cv WHERE cv.company_slug = $1 AND cv.vote =  1),
    downvote_count = (SELECT count(*) FROM company_votes cv WHERE cv.company_slug = $1 AND cv.vote = -1)
WHERE slug = $1
RETURNING upvote_count, downvote_count;
