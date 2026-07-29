## Why

The account-pre-hijacking defence has a credential it never revokes. When a provider-verified
OAuth identity arrives for an unverified, password-backed account, `SeizeUnverifiedAccount`
clears the password and bumps `token_version` — but an API key does not authenticate through
`token_version`. `AuthenticateAPIKey` matches a presented token against `api_keys.token_hash`
and `expires_at` alone and never joins `users`, so nothing about the seizure reaches it. The
only `DELETE FROM api_keys` in the codebase is the owner-scoped single-key revoke.

That the two are decoupled is deliberate and specified: `user-auth` carries the scenario "API
keys survive a session revocation", so a `logout-all` leaves a user's CLI credential working.
What the rule does not distinguish is a *session revocation* from a *change of account holder*.

The gap is reachable end to end. Registration issues a session cookie before the address is
proven, `POST /me/api-keys` has no verified-address gate, and the mint hard-codes `scope=full`
with `expires_at` NULL when the body omits it. So a squatter who registers `victim@corp.com`
takes away a never-expiring, full-scope bearer credential; the victim's later Google sign-in
destroys the password and the squatter's cookie, and leaves that key untouched. It reaches the
91 routes behind `RequireAuthOrKey` — including `/me/inbox` and `/me/emails/:id` (the victim's
mail), `/me/cvs/:id/pdf`, `/me/profile`, `/me/experience`, `/me/tracking*`, `/me/credits`.

The same hole sits behind `ResetUserPassword`. A reset by mailed code is how an owner takes an
account back after a compromise; leaving keys minted under the old password alive makes the
recovery cosmetic.

## What Changes

- **A seizure and a password reset destroy the account's API keys.** Both statements gain a
  data-modifying CTE (`WITH revoked_keys AS (DELETE FROM api_keys WHERE user_id = $1)`), so the
  revocation is welded to the takeover rather than left to a call site to remember. Postgres
  runs such a CTE exactly once and to completion whether or not the outer query reads it.
- **`logout-all` and a password change are unchanged.** They are the account holder's own
  actions, not a change of holder; the existing "API keys survive a session revocation" rule
  keeps a user's CLI and agent integrations working across "sign out everywhere". The rule is
  narrowed to say which events are the exception.
- **An account whose address was never proven cannot mint an API key.** `CreateAPIKey` becomes
  `INSERT ... SELECT ... WHERE users.id = $1 AND users.email_verified`; no row inserted means
  `403`. This closes the vector at the issuing end, so it does not depend on a future revocation
  path remembering `api_keys` — and no call site can mint around it.

## Impact

- Specs: `user-auth` (identity resolution, global session revocation), `password-recovery`
  (reset by code), `api-keys` (creating a key).
- Code: `internal/db/queries/users.sql`, `internal/db/queries/api_keys.sql` (+ `make sqlc`),
  `internal/handler/api_keys.go`.
- No migration. `CreateAPIKeyParams` is unchanged, so no call site churns.
- Behaviour change for clients: `POST /api/v1/me/api-keys` now answers `403` for an unverified
  account. Migration `0041` grandfathered every pre-existing account as verified, so only
  accounts registered by password and not yet confirmed are affected. The SPA already reads
  `email_verified` from `/auth/me`.
