-- Company feedback: one signed-in user's star rating + category + text per
-- (company, category) — a user may hold one review per category per company,
-- upserted in place (edit-by-resubmit) within that category, but may leave a
-- second review on the same company under a different category. LEFT JOIN
-- community_personas exactly like the community.sql read paths — content
-- outlives its author (a deleted account leaves user_id NULL), so a null
-- handle means "no live author", which the API renders as a deleted-member
-- marker.

-- name: UpsertCompanyFeedback :one
-- Insert or overwrite the caller's feedback in one category on a company in
-- place — the same upsert-in-place shape as UpsertCompanyVote, but ON
-- CONFLICT names the target partial index directly since the unique index
-- excludes NULL user_id.
INSERT INTO company_feedback (user_id, company_slug, rating, feedback_type, body)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, company_slug, feedback_type) WHERE user_id IS NOT NULL DO UPDATE
  SET rating = EXCLUDED.rating, body = EXCLUDED.body, updated_at = now()
RETURNING *;

-- name: DeleteCompanyFeedback :exec
-- Remove the caller's own feedback in one category on a company (no-op when absent).
DELETE FROM company_feedback WHERE user_id = $1 AND company_slug = $2 AND feedback_type = $3;

-- name: GetMyCompanyFeedback :one
-- The caller's own feedback in one category on a company, for the edit form's
-- prefill and for Upsert to tell an edit from a genuinely new review. No row
-- when they have not left one in that category yet. Not filtered by status:
-- the owner can still see and edit their own review after a moderator hides it.
SELECT * FROM company_feedback WHERE user_id = $1 AND company_slug = $2 AND feedback_type = $3;

-- name: ListMyCompanyFeedback :many
-- All of the caller's own feedback on a company, across every category they've
-- reviewed it under — the write dialog's "which categories have I already
-- used" read. Not filtered by status, same reasoning as GetMyCompanyFeedback.
SELECT * FROM company_feedback WHERE user_id = $1 AND company_slug = $2
ORDER BY created_at DESC, id DESC;

-- name: ListCompanyFeedback :many
-- A company's feedback, newest first, offset-paginated — the volume per company
-- is small (bounded by distinct reviewers), so unlike the discussion threads this
-- does not need keyset paging. Hidden rows (a moderator's lever) are excluded,
-- same as an open thread listing excludes closed ones.
SELECT f.*, p.handle AS author_handle
FROM company_feedback f
LEFT JOIN community_personas p ON p.user_id = f.user_id
WHERE f.company_slug = $1 AND f.status = 'visible'
ORDER BY f.created_at DESC, f.id DESC
LIMIT $2 OFFSET $3;

-- name: CountCompanyFeedback :one
SELECT count(*) FROM company_feedback WHERE company_slug = $1 AND status = 'visible';

-- name: RecountCompanyFeedback :one
-- Recompute a single company's materialized feedback_count/feedback_rating_avg
-- from company_feedback and return them. Run as its own statement AFTER the
-- write within one transaction, scoped to one company_slug via
-- company_feedback_company_slug_idx — the same shape as RecountCompanyVotes.
-- Hidden rows don't count toward either number.
UPDATE companies SET
    feedback_count      = (SELECT count(*) FROM company_feedback f WHERE f.company_slug = $1 AND f.status = 'visible'),
    feedback_rating_avg = (SELECT avg(rating) FROM company_feedback f WHERE f.company_slug = $1 AND f.status = 'visible')
WHERE slug = $1
RETURNING feedback_count, feedback_rating_avg;

-- name: CountRecentCompanyFeedback :one
-- How many new reviews (new company, or a new category on an already-reviewed
-- company) this user has left since `since` — the rate-limit check backing
-- companyfeedback.Service.checkRate (mirrors community.CountRecentThreads). An
-- edit-by-resubmit never inserts a new row (the ON CONFLICT DO UPDATE branch
-- leaves created_at untouched), so this only grows on genuinely new rows.
SELECT count(*) FROM company_feedback WHERE user_id = $1 AND created_at >= $2;

-- name: GetCompanyFeedbackSlug :one
-- The company_slug for a review id, read (unlocked) BEFORE HideCompanyFeedback so
-- Service.Hide can take LockCompanyForVote before it touches the feedback row —
-- the same company-then-feedback lock order Upsert/Delete already use, needed to
-- avoid a lock-order deadlock with those paths. pgx.ErrNoRows on an unknown id.
SELECT company_slug FROM company_feedback WHERE id = $1;

-- name: HideCompanyFeedback :one
-- The moderator lever: hide a specific review (idempotent — hiding an already-
-- hidden row is a no-op). Returns the company_slug so the caller can recompute
-- that company's counters in the same transaction. pgx.ErrNoRows on an unknown id.
UPDATE company_feedback SET status = 'hidden', updated_at = now()
WHERE id = $1
RETURNING company_slug;

-- name: CompanyFeedbackExists :one
-- The cheap existence check Report uses before writing, so a bad id 404s
-- cleanly instead of surfacing company_feedback_reports' FK violation.
SELECT EXISTS(SELECT 1 FROM company_feedback WHERE id = $1);

-- name: InsertCompanyFeedbackReport :exec
-- File a report against a specific review. A second report by the same user on
-- the same review is a silent no-op (the partial unique index) — evidence for a
-- moderator to act on, not a per-report ticket with its own state machine (see
-- internal/report for that fuller shape, built for job postings).
INSERT INTO company_feedback_reports (feedback_id, reporter_user_id, reason)
VALUES ($1, $2, $3)
ON CONFLICT (feedback_id, reporter_user_id) WHERE reporter_user_id IS NOT NULL DO NOTHING;

-- name: ListReportedCompanyFeedback :many
-- The moderator view: every review with at least one report, most-reported
-- first, with the report count and the distinct reasons given so a moderator
-- can triage without opening each report individually.
SELECT f.*, p.handle AS author_handle,
       count(r.id) AS report_count,
       array_agg(DISTINCT r.reason)::text[] AS report_reasons
FROM company_feedback f
JOIN company_feedback_reports r ON r.feedback_id = f.id
LEFT JOIN community_personas p ON p.user_id = f.user_id
GROUP BY f.id, p.handle
ORDER BY count(r.id) DESC, max(r.created_at) DESC;
