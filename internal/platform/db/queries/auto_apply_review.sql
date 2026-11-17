-- name: MarkAutoApplyTailorFailed :one
-- Records that a tailoring run gave up without producing a CV (migration 0154), so the
-- entry stops reading as one still being prepared.
--
-- Guarded by tailored_cv_id IS NULL as well as review_decision IS NULL: a run that failed
-- AFTER an earlier one had already produced a CV has nothing to report — the candidate has
-- something to look at, and telling them preparation failed while it sits there ready is
-- worse than saying nothing. DeriveStatus ranks the same way and does not depend on this
-- guard having fired.
--
-- Returns the job's title/company/slug for the caller's own notification, the same shape and
-- for the same reason SetAutoApplyResolvedPreview already returns them: this statement
-- already holds the job_id its own WHERE resolved, and a second round trip for exactly what
-- this write just touched would be a query with no reason to exist. pgx.ErrNoRows means a
-- guard fired, which the caller treats as "nothing to say", never as an error.
WITH updated AS (
    UPDATE auto_apply_queue q
    SET tailor_failed_at = now(),
        last_error       = sqlc.arg(last_error)
    WHERE q.id = sqlc.arg(id)
      AND q.review_decision IS NULL
      AND q.tailored_cv_id IS NULL
    RETURNING q.job_id, q.user_id
)
SELECT j.public_slug, j.title, j.company, u.user_id
FROM jobs j
JOIN updated u ON u.job_id = j.id;

-- name: GetAutoApplyQueueEntryForReview :one
-- One read backing both the tailoring-trigger and the review-decision endpoints
-- (openspec/changes/auto-apply-tailored-resume): resolves ownership (a foreign or missing
-- id comes back as pgx.ErrNoRows, which the handler renders as 404 — never 403, so a
-- probing caller learns nothing about entries they do not own) and carries enough of the
-- entry's own state (job_id for tailoring, tailored_cv_id/review_decision for the review
-- gate) that neither endpoint needs a second query to decide whether to proceed.
SELECT id, user_id, job_id, tailored_cv_id, review_decision
FROM auto_apply_queue
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: SetAutoApplyTailoredCV :execrows
-- Records which tailored CV a queue entry's tailoring run produced. Guarded by
-- review_decision IS NULL, matching ApproveAutoApplyReview/DeclineAutoApplyReview's own
-- guard: PostAutoApplyTailor's own review_decision check happens before its (potentially
-- minutes-long) LLM run, not after, so a candidate can approve or decline an EARLIER
-- tailored CV while a stale or retried tailor call for the same entry is still in flight.
-- Without this guard that call's own write here would silently attach a fresh, never-seen
-- CV to an already-decided entry — which ClaimAutoApplyBatch's own predicate
-- (tailored_cv_id IS NOT NULL AND review_decision = 'approved') would then submit for
-- real. Zero rows here means exactly that race happened; the handler checks it.
--
-- Also clears resolved_preview and resets the preview pass's own attempts/failed_at
-- (migration 0140): a deliberate re-tailor (this entry already had a tailored_cv_id) means
-- the candidate's profile or the tailoring evidence changed, so a preview computed against
-- the PREVIOUS CV is stale and must be recomputed — leaving it would let the candidate
-- review an answer preview that no longer matches what would actually be submitted. Reset,
-- not left exhausted: an entry whose preview pass had already dead-lettered deserves a
-- fresh attempt budget against the fresh CV, not a permanent unclaimable state.
UPDATE auto_apply_queue
SET tailored_cv_id    = sqlc.arg(tailored_cv_id),
    resolved_preview  = NULL,
    preview_attempts  = 0,
    preview_failed_at = NULL,
    tailor_failed_at  = NULL
WHERE id = sqlc.arg(id) AND review_decision IS NULL;

-- name: ApproveAutoApplyReview :execrows
-- Records an approval. Guarded by review_decision IS NULL so a second attempt at an
-- already-reviewed entry affects zero rows rather than overwriting a recorded decision —
-- the handler reads the row first (GetAutoApplyQueueEntryForReview) to tell "already
-- reviewed" apart from "not found" before ever reaching this statement, so zero rows here
-- would only mean a race with a concurrent decision on the same entry.
UPDATE auto_apply_queue
SET review_decision = 'approved',
    reviewed_at     = now()
WHERE id = sqlc.arg(id) AND review_decision IS NULL;

-- name: DeclineAutoApplyReview :execrows
-- Records a decline AND parks the entry in one statement — the same fields
-- MarkAutoApplyBlocked sets (blocked_at, last_error), reusing that park vocabulary rather
-- than inventing a second one, plus the review columns MarkAutoApplyBlocked has no reason
-- to know about. unmapped stays NULL: this is not a form-field park. last_error is what
-- tells the two park reasons apart in the queue's own history, per design.md.
UPDATE auto_apply_queue
SET review_decision = 'declined',
    reviewed_at     = now(),
    blocked_at      = now(),
    last_error      = sqlc.arg(last_error)
WHERE id = sqlc.arg(id) AND review_decision IS NULL;

-- name: MarkStaleAutoApplyTailoringFailed :execrows
-- Gives up on attempts whose tailoring nobody is doing: no CV, no failure recorded, no
-- decision, and nothing has happened to the pair for stale_seconds. The orchestrator is
-- started by a best-effort publish and a run can die between steps; either way the entry sat
-- reading "tailoring" forever, unclaimable by both passes, with no failure for a retry to act
-- on (two production entries since 2026-09-07). Marking it tailor_failed is what makes it
-- visible and retryable — and the trigger records the event.
--
-- "Nothing has happened" is the pair's newest history event, not created_at: a retry
-- re-publishes the orchestrator for an old entry, and measuring from created_at would fail it
-- again before the new run could finish. COALESCE covers entries older than the history.
UPDATE auto_apply_queue q
SET tailor_failed_at = now(),
    last_error       = 'tailoring never finished'
WHERE q.tailored_cv_id IS NULL
  AND q.tailor_failed_at IS NULL
  AND q.review_decision IS NULL
  AND q.blocked_at IS NULL
  AND q.failed_at IS NULL
  AND COALESCE((SELECT max(e.created_at) FROM auto_apply_events e
                WHERE e.user_id = q.user_id AND e.job_id = q.job_id), q.created_at)
      < now() - make_interval(secs => sqlc.arg(stale_seconds)::bigint);
