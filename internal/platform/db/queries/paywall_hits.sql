-- Queries for the paywall-hit record (migration 0248): every refusal of a free user by the
-- paid-AI gate, written by plan.Store.RecordPaywallHit.

-- name: RecordPaywallHit :exec
-- Records that the paid-AI gate refused this user on this feature today, once per
-- (user, feature, job, day) — a retried request or an iteration over slugs adds nothing the
-- first row did not say. The vacancy is resolved here from whichever identifier the route
-- carries, as a bound parameter, so no request text is stored and no extra round trip is
-- made: a job slug, or the CV or assistant session the route is about (both scoped to the
-- caller, so another user's id resolves to nothing; a private job resolves only for its creator,
-- so a leaked slug never puts its title in another account's letter). An unknown identifier
-- leaves job_id NULL.
INSERT INTO paywall_hits (user_id, feature, job_id, day)
VALUES (
    sqlc.arg(user_id),
    sqlc.arg(feature)::text,
    COALESCE(
        (SELECT j.id FROM jobs j WHERE j.public_slug = sqlc.narg(job_slug)::text
           AND (NOT j.is_private OR j.created_by = sqlc.arg(user_id))),
        (SELECT c.job_id FROM cvs c WHERE c.id = sqlc.narg(cv_id)::uuid AND c.user_id = sqlc.arg(user_id)),
        (SELECT s.job_id FROM assistant_sessions s WHERE s.id = sqlc.narg(session_id)::uuid AND s.user_id = sqlc.arg(user_id))
    ),
    sqlc.arg(day)
)
ON CONFLICT (user_id, feature, COALESCE(job_id, 0), day) DO NOTHING;

-- name: ListPaywallHitsByFeature :many
-- Who hit which paywall in the last window_days, by feature and newest first: the operator
-- report. job_slug is NULL when the action concerned no vacancy, or the vacancy is gone.
SELECT h.feature, h.user_id, j.public_slug AS job_slug, h.day, h.created_at
FROM paywall_hits h
LEFT JOIN jobs j ON j.id = h.job_id
WHERE h.created_at >= now() - make_interval(days => sqlc.arg(window_days)::int)
ORDER BY h.feature, h.created_at DESC, h.id DESC;
