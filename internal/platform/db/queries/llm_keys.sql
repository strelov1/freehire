-- name: GetUserLLMKey :one
-- The gateway credential this account is known by, or "" when none has been minted.
-- coalesce keeps the empty string as the single "not minted" signal, so callers test a
-- string rather than unwrapping a nullable through pgtype on every model call.
--
-- The id comes back beside the secret because the two are one credential: the secret is
-- what a model call presents, the id is what an administrative call addresses. A reader
-- that took only the secret could spend but never revoke. The id is separately empty for
-- a credential minted before 0119 — a real state, not a fault; see the migration.
SELECT coalesce(llm_key, '')::text AS llm_key,
       coalesce(llm_key_id, '')::text AS llm_key_id
FROM users
WHERE id = $1;

-- name: ClaimUserLLMKey :one
-- Store a freshly minted credential, but only if this account still has none, and report
-- what is now stored. NO ROWS means somebody else claimed first — the caller re-reads the
-- winner's credential and deletes the one it minted, which would otherwise sit at the
-- gateway forever spending nothing and appearing in every listing.
--
-- The `IS NULL` guard is the whole race resolution: two concurrent first calls both mint,
-- the row lock serializes them, and the loser's UPDATE re-evaluates the guard against the
-- committed row and matches nothing.
--
-- The guard stays on llm_key alone. That column is what decides whether an account has a
-- credential at all; guarding on both would let a pre-0119 row — secret set, id null —
-- read as unclaimed and be overwritten, orphaning a key that is still spending.
UPDATE users
SET llm_key = sqlc.arg(llm_key),
    llm_key_id = sqlc.arg(llm_key_id)
WHERE id = sqlc.arg(id) AND llm_key IS NULL
RETURNING coalesce(llm_key, '')::text AS llm_key,
          coalesce(llm_key_id, '')::text AS llm_key_id;

-- name: ClearUserLLMKey :exec
-- Forget a credential the gateway no longer recognises, so the next call mints a
-- replacement. Conditional on the value we believe is stored: a concurrent call may have
-- already re-minted, and clearing unconditionally would throw away that good credential
-- and leave it orphaned at the gateway.
--
-- Both columns clear together. An id left behind would point at a credential nothing can
-- present, and the next mint would then fail a UNIQUE constraint on a value nobody can
-- explain.
UPDATE users
SET llm_key = NULL,
    llm_key_id = NULL
WHERE id = sqlc.arg(id) AND llm_key = sqlc.arg(llm_key);
