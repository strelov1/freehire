-- name: GetCoverLetter :one
-- The caller's current cover letter for one vacancy, with the two staleness stamps it was
-- written against: model and language. No row means the pair was never drafted, and the
-- handler reports that without calling a model. The handler compares both stamps against
-- the live model and the vacancy's posting_language to decide the stale flag — note the
-- language stamp is checked against the VACANCY, not the caller's profile, unlike
-- GetUserJobAnalysis.
SELECT body, cited_atom_ids, language, model, created_at, updated_at, evidence_revision, evidence_fingerprint
FROM cover_letters
WHERE user_id = $1 AND job_id = $2;

-- name: UpsertCoverLetter :exec
-- Create-or-replace the current letter for a (user, job). The composite PRIMARY KEY makes
-- it idempotent: drafting again overwrites the body, the cited atoms and both stamps.
-- There is one row per pair and no history — see the migration for why a letter does not
-- earn cvedit's revisions.
--
-- created_at is NOT re-bumped on conflict, so it records when this pair was FIRST drafted;
-- updated_at moves every time. Nothing meters on created_at today, but a letter that
-- re-ages itself on every redraft would silently break any future rule that did — the same
-- trap the fit-analysis quota documents on its own upsert.
INSERT INTO cover_letters (user_id, job_id, body, cited_atom_ids, language, model, created_at, updated_at, evidence_revision, evidence_fingerprint)
VALUES ($1, $2, $3, $4, $5, $6, now(), now(), $7, $8)
ON CONFLICT (user_id, job_id) DO UPDATE
SET body           = EXCLUDED.body,
    cited_atom_ids = EXCLUDED.cited_atom_ids,
    language       = EXCLUDED.language,
    model          = EXCLUDED.model,
 evidence_revision = EXCLUDED.evidence_revision,
 evidence_fingerprint = EXCLUDED.evidence_fingerprint,
    updated_at     = now();

-- name: GetCoverLetterEvidence :one
SELECT revision, atom_ids FROM cover_letter_evidence WHERE user_id = $1 AND job_id = $2;

-- name: SaveCoverLetterEvidence :one
INSERT INTO cover_letter_evidence (user_id,job_id,revision,atom_ids)
SELECT sqlc.arg(user_id),sqlc.arg(job_id),1,sqlc.arg(atom_ids)::uuid[]
WHERE sqlc.arg(expected_revision)::bigint = 0
ON CONFLICT (user_id,job_id) DO NOTHING
RETURNING revision,atom_ids;

-- name: UpdateCoverLetterEvidence :one
UPDATE cover_letter_evidence SET revision = revision + 1, atom_ids = sqlc.arg(atom_ids)::uuid[]
WHERE user_id = sqlc.arg(user_id) AND job_id = sqlc.arg(job_id)
 AND revision = sqlc.arg(expected_revision)::bigint
RETURNING revision,atom_ids;
