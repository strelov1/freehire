-- name: UpsertEmailCode :exec
-- Issue (or replace) the outstanding code for one purpose. The composite primary key
-- makes this the whole "at most one live code per (user, purpose)" rule: a resend
-- overwrites the hash and expiry and resets attempts, so a burnt code never lingers
-- beside a fresh one. created_at is re-stamped, which is what the resend cooldown reads.
INSERT INTO user_email_codes (user_id, purpose, code_hash, expires_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (user_id, purpose) DO UPDATE
SET code_hash  = EXCLUDED.code_hash,
    expires_at = EXCLUDED.expires_at,
    attempts   = 0,
    created_at = now();

-- name: GetEmailCode :one
-- The outstanding code for a purpose. No row means nothing was issued or it was consumed;
-- the caller treats pgx.ErrNoRows as "request a new code". Expiry and the attempt ceiling
-- are enforced by the caller, which must fail identically for expired and wrong codes.
SELECT code_hash, expires_at, attempts, created_at
FROM user_email_codes
WHERE user_id = $1 AND purpose = $2;

-- name: GetEmailCodeForUpdate :one
-- The outstanding code for a purpose locked for update inside a transaction.
SELECT code_hash, expires_at, attempts, created_at
FROM user_email_codes
WHERE user_id = $1 AND purpose = $2
FOR UPDATE;


-- name: BumpEmailCodeAttempts :one
-- Count one failed confirmation and return the new total, so the caller can burn the code
-- once it reaches the ceiling. Atomic, so concurrent guesses cannot share an attempt.
UPDATE user_email_codes
SET attempts = attempts + 1
WHERE user_id = $1 AND purpose = $2
RETURNING attempts;

-- name: DeleteEmailCode :exec
-- Consume the code (on success) or burn it (on too many attempts). Idempotent: deleting
-- an absent code is a no-op, so a double submit cannot fail the request.
DELETE FROM user_email_codes
WHERE user_id = $1 AND purpose = $2;
