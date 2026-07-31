-- name: UpsertTracerLink :one
-- Mint the token for one traced link, or return the token that link already has.
--
-- The PDF is not stored: it is re-rendered on every download, so this runs on every download too.
-- Idempotency is therefore not an optimisation but the condition for the feature working at all —
-- without it three downloads would produce three tokens for one link and scatter the counts.
--
-- The no-op DO UPDATE (writing cv_id its own value) is what makes the token come back on conflict:
-- ON CONFLICT DO NOTHING returns no row, so the caller would have to read the token in a second
-- statement and race with a concurrent download doing the same. Same idiom as UpsertJob.
--
-- Owner-scoped through the CV: the SELECT yields nothing for a CV the caller does not own, so the
-- INSERT writes nothing rather than minting a token against a stranger's CV.
INSERT INTO cv_tracer_links (cv_id, token, source_path, destination_url, destination_hash)
SELECT c.id, sqlc.arg(token), sqlc.arg(source_path), sqlc.arg(destination_url), sqlc.arg(destination_hash)
FROM cvs c
WHERE c.id = sqlc.arg(cv_id) AND c.user_id = sqlc.arg(user_id)
ON CONFLICT (cv_id, source_path, destination_hash) DO UPDATE SET cv_id = EXCLUDED.cv_id
RETURNING token;

-- name: TracerLinkByToken :one
-- Resolve a token to where the visitor must be sent, plus what the click needs to be attributed:
-- the link's own id and the id of the user whose CV it belongs to (so a click by that user can be
-- marked as the owner's own). Unauthenticated read — the token IS the credential, and it grants
-- nothing but a redirect.
SELECT l.id, l.destination_url, c.user_id AS owner_id
FROM cv_tracer_links l
JOIN cvs c ON c.id = l.cv_id
WHERE l.token = $1;

-- name: RecordTracerClick :exec
-- Write one click. Best-effort by contract: the handler redirects whether or not this succeeds,
-- because a broken redirect lives in a PDF the candidate can neither see nor fix.
INSERT INTO cv_link_clicks (
    tracer_link_id, is_likely_bot, is_owner, device_type, os_family, ua_family, referrer_host, visitor_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: TouchCVLastClick :exec
-- Stamp the CV a click belongs to, for the tracking board's "CV opened" marker. Issued right after
-- the click insert, as a separate statement rather than in one transaction with it: both writes are
-- best-effort behind a redirect that must happen regardless, so there is nothing for a rollback to
-- protect. Whether a click counts is decided by the caller, not here, so this stays a plain stamp.
--
-- GREATEST guards against an out-of-order write moving the marker backwards.
UPDATE cvs c
SET last_click_at = GREATEST(c.last_click_at, now())
FROM cv_tracer_links l
WHERE l.id = $1 AND c.id = l.cv_id;

-- name: ListTracerLinkStats :many
-- The owner's per-CV panel: every traced link of one CV with what is known about it. Owner-scoped.
--
-- Clicks flagged as automated are counted separately rather than filtered out, so the UI's "include
-- likely bots" switch needs no second query. The owner's own clicks are excluded from every count
-- here: they are recorded so the history is complete, not so they can be reported back as somebody
-- having opened the CV.
--
-- distinct_visitors counts non-empty hashes only. An empty visitor_hash means the deployment has no
-- salt configured, and counting those rows would report every unidentifiable click as one visitor.
SELECT l.token, l.source_path, l.destination_url, l.created_at,
       count(k.id) FILTER (WHERE NOT k.is_likely_bot AND NOT k.is_owner)                       AS clicks,
       count(k.id) FILTER (WHERE k.is_likely_bot AND NOT k.is_owner)                           AS bot_clicks,
       count(DISTINCT k.visitor_hash) FILTER (
           WHERE NOT k.is_likely_bot AND NOT k.is_owner AND k.visitor_hash <> '')              AS distinct_visitors,
       -- Cast so sqlc gives this a timestamp type: an aggregate inside FILTER has no inferable
       -- one, and the generated field would be interface{}.
       (max(k.clicked_at) FILTER (WHERE NOT k.is_likely_bot AND NOT k.is_owner))::timestamptz AS last_click_at
FROM cv_tracer_links l
JOIN cvs c ON c.id = l.cv_id
LEFT JOIN cv_link_clicks k ON k.tracer_link_id = l.id
-- Filtered on the CV row rather than on l.cv_id: same rows, but it keeps the caller's cv_id one
-- Go type across every query here. Matching the nullable foreign key instead makes sqlc infer a
-- nullable UUID, and the handler would carry two spellings of one id.
WHERE c.id = sqlc.arg(cv_id) AND c.user_id = sqlc.arg(user_id)
GROUP BY l.id, l.token, l.source_path, l.destination_url, l.created_at
ORDER BY l.created_at, l.source_path;

-- name: DeleteExpiredTracerClicks :execrows
-- The 180-day retention sweep, run by cmd/prune — the repository's single hard-delete path. The
-- tokens themselves are kept: an old PDF must keep redirecting even once the clicks behind it have
-- aged out.
DELETE FROM cv_link_clicks WHERE clicked_at < now() - sqlc.arg(max_age)::interval;

-- name: SetCVTracerLinks :execrows
-- Turn link tracing on or off for one CV. Owner-scoped: a foreign or missing id matches no row,
-- and the handler renders that as a 404.
--
-- Deliberately not routed through cvedit, which is otherwise the only writer of a stored CV. Every
-- write there becomes a revision with a computed inverse, and a consent to track a third party
-- must not be something an undo of an unrelated edit can grant or revoke.
UPDATE cvs SET tracer_links_enabled = $3, updated_at = now()
WHERE id = $1 AND user_id = $2;

-- name: CountExpiredTracerClicks :one
-- What the retention sweep would remove. cmd/prune reports before it deletes, and a dry run that
-- cannot say a number is not a report.
SELECT count(*) FROM cv_link_clicks WHERE clicked_at < now() - sqlc.arg(max_age)::interval;
