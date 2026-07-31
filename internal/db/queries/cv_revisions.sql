-- name: GetCVForEdit :one
-- Read an owned CV's editable state and lock the row for the rest of the transaction, so a
-- commit reads, applies and records against a document nobody else is changing underneath it.
-- The lock is what serialises edits to one CV: two agent turns arriving together used to
-- interleave, and the pre-run snapshot each took was of a half-edited document. Owner-scoped:
-- no row for a foreign or missing id.
SELECT id, user_id, title, template_id, data, updated_at
FROM cvs
WHERE id = $1 AND user_id = $2
FOR UPDATE;

-- name: InsertCVRevision :one
-- Record one change: what it did (ops), what would undo it (inverse), who made it and through
-- which entry point, and the document version it was computed against. Written in the same
-- transaction as the document it changed — a change without its revision, or a revision
-- without its change, would make the feed lie.
INSERT INTO cv_revisions (cv_id, user_id, actor, origin, batch_id, title, note, ops, inverse, base_version, reverts_id)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: AmendCVRevision :one
-- Fold a follow-on edit into the newest revision: replace what it does and restate its
-- description, but LEAVE inverse alone. The inverse still leads back to the state before the
-- first of the coalesced edits, which is what makes undo mean something for typed text.
UPDATE cv_revisions
SET ops = $3, title = $4, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: NewestCVRevision :one
-- The revision a follow-on edit might be folded into. Only the newest is a candidate:
-- coalescing into anything older would reorder the log.
SELECT *
FROM cv_revisions
WHERE cv_id = $1 AND user_id = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: GetCVRevision :one
-- One revision of one CV — what undo reads to find the inverse it must apply. Scoped by BOTH
-- the owner and the CV: a revision id names an entry in one history, and reading it through a
-- different CV of the same owner would undo the wrong document.
SELECT *
FROM cv_revisions
WHERE id = $1 AND user_id = $2 AND cv_id = $3;

-- name: ListCVRevisions :many
-- The feed, newest first.
SELECT *
FROM cv_revisions
WHERE cv_id = $1 AND user_id = $2
ORDER BY created_at DESC
LIMIT $3;

-- name: ListCVRevisionsInBatch :many
-- Every revision of one agent turn or autopilot run that is still standing, newest first —
-- the order a whole-run revert must undo them in.
SELECT *
FROM cv_revisions
WHERE cv_id = $1 AND user_id = $2 AND batch_id = $3 AND reverted_at IS NULL
ORDER BY created_at DESC;

-- name: MarkCVRevisionReverted :execrows
-- Stamp a revision as undone. Guarded on reverted_at IS NULL so undoing twice affects no row
-- and the caller can tell the difference without a second read.
UPDATE cv_revisions
SET reverted_at = now(), updated_at = now()
WHERE id = $1 AND user_id = $2 AND reverted_at IS NULL;

-- name: TrimCVRevisions :execrows
-- Keep only the newest $2 revisions of a CV. A revision log is an aid to the candidate's
-- current work, not an archive, and each row carries two operation documents on the table
-- behind every CV page.
DELETE FROM cv_revisions old
WHERE old.cv_id = $1
  AND old.id NOT IN (
    SELECT keep.id FROM cv_revisions keep WHERE keep.cv_id = $1 ORDER BY keep.created_at DESC LIMIT $2
  );
