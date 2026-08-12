-- Company feedback: one signed-in user's star rating + category + text per
-- company, upserted in place (edit-by-resubmit). LEFT JOIN community_personas
-- exactly like the community.sql read paths — content outlives its author (a
-- deleted account leaves user_id NULL), so a null handle means "no live author",
-- which the API renders as a deleted-member marker.

-- name: UpsertCompanyFeedback :one
-- Insert or overwrite the caller's feedback on a company in place — the same
-- upsert-in-place shape as UpsertCompanyVote, but ON CONFLICT names the target
-- partial index directly since the unique index excludes NULL user_id.
INSERT INTO company_feedback (user_id, company_slug, rating, feedback_type, body)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, company_slug) WHERE user_id IS NOT NULL DO UPDATE
  SET rating = EXCLUDED.rating, feedback_type = EXCLUDED.feedback_type,
      body = EXCLUDED.body, updated_at = now()
RETURNING *;

-- name: DeleteCompanyFeedback :exec
-- Remove the caller's own feedback (no-op when absent).
DELETE FROM company_feedback WHERE user_id = $1 AND company_slug = $2;

-- name: GetMyCompanyFeedback :one
-- The caller's own feedback on a company, for the edit form's prefill. No row
-- when they have not left one yet.
SELECT * FROM company_feedback WHERE user_id = $1 AND company_slug = $2;

-- name: ListCompanyFeedback :many
-- A company's feedback, newest first, offset-paginated — the volume per company
-- is small (bounded by distinct reviewers), so unlike the discussion threads this
-- does not need keyset paging.
SELECT f.*, p.handle AS author_handle
FROM company_feedback f
LEFT JOIN community_personas p ON p.user_id = f.user_id
WHERE f.company_slug = $1
ORDER BY f.created_at DESC, f.id DESC
LIMIT $2 OFFSET $3;

-- name: CountCompanyFeedback :one
SELECT count(*) FROM company_feedback WHERE company_slug = $1;

-- name: RecountCompanyFeedback :one
-- Recompute a single company's materialized feedback_count/feedback_rating_avg
-- from company_feedback and return them. Run as its own statement AFTER the
-- write within one transaction, scoped to one company_slug via
-- company_feedback_company_slug_idx — the same shape as RecountCompanyVotes.
UPDATE companies SET
    feedback_count      = (SELECT count(*) FROM company_feedback f WHERE f.company_slug = $1),
    feedback_rating_avg = (SELECT avg(rating) FROM company_feedback f WHERE f.company_slug = $1)
WHERE slug = $1
RETURNING feedback_count, feedback_rating_avg;
