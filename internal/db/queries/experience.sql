-- name: ListExperienceEmployments :many
-- The caller's places of work, current roles first and most recent within that. Owner-scoped
-- by construction — another user's employments can never appear.
SELECT id, user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, created_at, updated_at, link
FROM experience_employments
WHERE user_id = $1
ORDER BY is_current DESC, period_start DESC, id;

-- name: GetExperienceEmployment :one
-- One employment owned by the caller. A foreign or missing id returns no row, which the
-- handler maps to 404 — so a probe cannot tell the two apart.
SELECT id, user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, created_at, updated_at, link
FROM experience_employments
WHERE id = $1 AND user_id = $2;

-- name: FindExperienceEmployment :one
-- Import's match: the caller's employment with this company and role, compared case-
-- insensitively because a CV, a chat and a form will each capitalise them differently.
-- There is no unique constraint behind this on purpose (a second stint at the same employer
-- in the same role is a real career shape), so the oldest match wins and stays stable.
SELECT id, user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, created_at, updated_at, link
FROM experience_employments
WHERE user_id = @user_id AND lower(company) = lower(@company) AND lower(role) = lower(@role)
ORDER BY created_at, id
LIMIT 1;

-- name: CreateExperienceEmployment :one
INSERT INTO experience_employments (user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, link)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING id, user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, created_at, updated_at, link;

-- name: UpdateExperienceEmployment :one
-- A full owner-scoped replacement, used by the profile UI where the user is editing the
-- fields directly and means what they typed — including blanking one.
UPDATE experience_employments
SET kind = $3, company = $4, role = $5, location = $6, period_start = $7, period_end = $8,
    is_current = $9, summary = $10, stack = $11, link = $12, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING id, user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, created_at, updated_at, link;

-- name: FillExperienceEmploymentBlanks :one
-- Import's write: fill only the fields the bank has nothing for, and never overwrite a value
-- already there. A user who corrected their job title must not have that correction undone by
-- re-uploading the CV it came from. is_current is not touched at all — a CV that still says
-- "Present" for a role the user has left would otherwise resurrect it.
UPDATE experience_employments
SET company      = coalesce(nullif(company, ''), @company),
    role         = coalesce(nullif(role, ''), @role),
    location     = coalesce(nullif(location, ''), @location),
    period_start = coalesce(nullif(period_start, ''), @period_start),
    period_end   = coalesce(nullif(period_end, ''), @period_end),
    summary      = coalesce(nullif(summary, ''), @summary),
    link         = coalesce(nullif(link, ''), @link),
    -- The stack is unioned, not filled-if-blank: a CV listing one more technology for a
    -- role is new knowledge, and import must never take a technology away. coalesce
    -- guards the empty case — array_agg over no rows is NULL, and the column is NOT NULL.
    stack        = coalesce(
                       (SELECT array_agg(DISTINCT s ORDER BY s) FROM unnest(stack || @stack::text[]) AS s),
                       '{}'::text[]
                   ),
    updated_at   = now()
WHERE id = @id AND user_id = @user_id
RETURNING id, user_id, kind, company, role, location, period_start, period_end, is_current, summary, stack, created_at, updated_at, link;

-- name: DeleteExperienceEmployment :execrows
-- Remove an owned employment; its atoms go with it (ON DELETE CASCADE) because they are
-- evidence OF that role. Returns 0 affected rows for a foreign or missing id.
DELETE FROM experience_employments
WHERE id = $1 AND user_id = $2;

-- name: ListExperienceAtoms :many
-- Every atom the caller owns. Retrieval reads the whole set and scores it in Go: a
-- requirement can match on skills OR on text alone, so there is no prefilter that would not
-- drop real evidence. Ordered by employment so a consumer can group without a second pass.
SELECT id, user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref, created_at, updated_at
FROM experience_atoms
WHERE user_id = $1
ORDER BY employment_id NULLS LAST, created_at, id;

-- name: GetExperienceAtom :one
SELECT id, user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref, created_at, updated_at
FROM experience_atoms
WHERE id = $1 AND user_id = $2;

-- name: InsertExperienceAtomIfNew :one
-- The only insert. A claim already banked with a stronger provenance than
-- agent_inferred is never touched — ON CONFLICT DO NOTHING behavior for every case
-- except one: a claim first recorded as agent_inferred (the model's unconfirmed
-- paraphrase) is upgraded in place when a later call carries a real provenance,
-- because the candidate confirming it afterward must actually unstick the write —
-- otherwise that exact claim text stays permanently un-writable to a CV. The WHERE
-- guards both non-upgrade cases: a confirmed atom is never downgraded, and two
-- agent_inferred attempts at the same claim leave it exactly as unconfirmed as
-- before. Returns no row when there is genuinely nothing to change, which callers
-- report as ErrAlreadyBanked rather than an error.
INSERT INTO experience_atoms (user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, claim_key) DO UPDATE
SET provenance = EXCLUDED.provenance,
    source_ref = EXCLUDED.source_ref,
    context = EXCLUDED.context,
    updated_at = now()
WHERE experience_atoms.provenance = 'agent_inferred'
  AND EXCLUDED.provenance != 'agent_inferred'
RETURNING id, user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref, created_at, updated_at;

-- name: UpdateExperienceAtom :one
-- Owner-scoped edit. claim_key moves with the claim, so the uniqueness guarantee holds after
-- an edit as well as after an insert.
UPDATE experience_atoms
SET employment_id = $3, claim = $4, claim_key = $5, context = $6, metrics = $7, skills = $8,
    provenance = $9, updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING id, user_id, employment_id, claim, claim_key, context, metrics, skills, provenance, source_ref, created_at, updated_at;

-- name: DeleteExperienceAtom :execrows
-- The only path that removes an atom, and it belongs to the user. Import never deletes.
DELETE FROM experience_atoms
WHERE id = $1 AND user_id = $2;

-- name: MergeExperienceAtoms :one
-- Atomically update the keep and delete the loser. The UPDATE is the transaction: the
-- DELETE only lands when the update did, so a keep that vanished between Store.MergeAtoms'
-- ownership check and this call (a concurrent delete/merge) yields no row and deletes
-- nothing, rather than deleting the loser out from under a merge whose other half never
-- landed. The UPDATE itself is gated on the loser still existing, for the same reason in
-- the other direction. Either both sides of the merge happen or neither does. Claim,
-- claim_key, employment_id and source_ref stay on the keep — only richness fields move.
--
-- Both rows are additionally gated on updated_at still matching what Store.MergeAtoms read
-- when it chose keep/lose and computed @context/@metrics/@skills: that computation happens
-- in Go, outside any transaction, so a write landing in the gap (an edit, another merge)
-- must not be silently clobbered by an UPDATE built from a stale snapshot. A mismatch here
-- yields no row exactly like a concurrent delete already did, and the caller reports both
-- the same way — reload and retry — rather than pretending a still-present row vanished.
WITH updated AS (
    UPDATE experience_atoms
    SET context = @context,
        metrics = @metrics,
        skills = @skills,
        provenance = @provenance,
        updated_at = now()
    WHERE experience_atoms.id = @keep_id AND experience_atoms.user_id = @user_id
      AND experience_atoms.updated_at = @keep_updated_at
      AND EXISTS (
          SELECT 1 FROM experience_atoms AS loser
          WHERE loser.id = @loser_id AND loser.user_id = @user_id
            AND loser.updated_at = @loser_updated_at
      )
    RETURNING experience_atoms.id, experience_atoms.user_id, experience_atoms.employment_id,
              experience_atoms.claim, experience_atoms.claim_key, experience_atoms.context,
              experience_atoms.metrics, experience_atoms.skills, experience_atoms.provenance,
              experience_atoms.source_ref, experience_atoms.created_at, experience_atoms.updated_at
),
deleted AS (
    DELETE FROM experience_atoms
    WHERE experience_atoms.id = @loser_id AND experience_atoms.user_id = @user_id
      AND EXISTS (SELECT 1 FROM updated)
    RETURNING experience_atoms.id
)
SELECT * FROM updated;

-- name: ListExperienceBackfillTargets :many
-- Every user with a stored CV, carrying their structured résumé ONLY when its stamp still
-- matches the upload time. That CASE is what makes the backfill cheap: a user whose
-- structure is current costs no model call, and one whose structure is stale or missing
-- falls through to extraction. The freshness test is the same one resume.Store.Structured
-- applies, so the worker never reuses a structure the app itself treats as absent.
SELECT id,
       resume_uploaded_at,
       CASE WHEN resume_structured_uploaded_at IS NOT DISTINCT FROM resume_uploaded_at
            THEN resume_structured
       END::jsonb AS current_structured
FROM users
WHERE resume_object_key IS NOT NULL
  AND resume_uploaded_at IS NOT NULL
  AND (@user_id::bigint = 0 OR id = @user_id::bigint)
ORDER BY id;
