-- name: GetSocialToken :one
-- The credential one channel publishes with. No rows means the channel has never been signed
-- in, which every caller reports as "not configured" rather than as an error — the same
-- degradation as an unset webhook URL.
SELECT channel, access_token, access_expires_at, refresh_token, refresh_expires_at,
       scope, obtained_at, refreshed_at
FROM social_tokens
WHERE channel = $1;

-- name: StoreSocialToken :exec
-- Record the credential a person has just signed in for.
--
-- obtained_at moves and refreshed_at is cleared, because both describe THIS grant: the
-- refresh token's 365-day life is measured from the sign-in and is not extended by a renewal,
-- so a stale obtained_at would make the warning arrive after the deadline it is warning about.
INSERT INTO social_tokens (channel, access_token, access_expires_at, refresh_token,
                           refresh_expires_at, scope, obtained_at, refreshed_at)
VALUES (sqlc.arg(channel), sqlc.arg(access_token), sqlc.arg(access_expires_at),
        sqlc.narg(refresh_token), sqlc.narg(refresh_expires_at), sqlc.arg(scope), now(), NULL)
ON CONFLICT (channel) DO UPDATE
SET access_token      = EXCLUDED.access_token,
    access_expires_at = EXCLUDED.access_expires_at,
    refresh_token     = EXCLUDED.refresh_token,
    refresh_expires_at = EXCLUDED.refresh_expires_at,
    scope             = EXCLUDED.scope,
    obtained_at       = now(),
    refreshed_at      = NULL;

-- name: RenewSocialToken :execrows
-- Write back what a refresh-token exchange returned.
--
-- obtained_at is deliberately NOT touched: the refresh token's clock keeps running from the
-- original sign-in, and a renewal that reset it would hide the one date that says when a
-- person must sign in again.
--
-- The refresh token is overwritten rather than preserved, because LinkedIn returns it on every
-- exchange and a provider that rotates it would otherwise leave us holding a dead one. It is
-- guarded on the row still existing (execrows), so a renewal racing a re-sign-in reports zero
-- rather than silently writing over a fresher grant... which it cannot do anyway, the WHERE
-- naming the access token this renewal was issued against.
UPDATE social_tokens
SET access_token       = sqlc.arg(access_token),
    access_expires_at  = sqlc.arg(access_expires_at),
    refresh_token      = sqlc.narg(refresh_token),
    refresh_expires_at = sqlc.narg(refresh_expires_at),
    refreshed_at       = now()
WHERE channel = sqlc.arg(channel)
  AND access_token = sqlc.arg(previous_access_token);
