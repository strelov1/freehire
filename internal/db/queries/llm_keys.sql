-- name: GetUserLLMKey :one
-- The gateway credential this account is known by, or "" when none has been minted.
-- coalesce keeps the empty string as the single "not minted" signal, so callers test a
-- string rather than unwrapping a nullable through pgtype on every model call.
SELECT coalesce(llm_key, '')::text AS llm_key
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
UPDATE users
SET llm_key = sqlc.arg(llm_key)
WHERE id = sqlc.arg(id) AND llm_key IS NULL
RETURNING coalesce(llm_key, '')::text AS llm_key;

-- name: ClearUserLLMKey :exec
-- Forget a credential the gateway no longer recognises, so the next call mints a
-- replacement. Conditional on the value we believe is stored: a concurrent call may have
-- already re-minted, and clearing unconditionally would throw away that good credential
-- and leave it orphaned at the gateway.
UPDATE users
SET llm_key = NULL
WHERE id = sqlc.arg(id) AND llm_key = sqlc.arg(llm_key);
