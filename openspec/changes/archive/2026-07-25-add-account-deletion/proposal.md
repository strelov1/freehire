## Why

A signed-in member can create an account, upload a CV, connect their Gmail, and accumulate mail, analyses, and referral proof — but has no way to leave. There is no self-serve deletion path at all, so the only exit is asking a human to run SQL, which neither the member nor we can verify. Members handing us their CV and mailbox deserve a one-click, honest exit.

## What Changes

- New cookie-only endpoint `DELETE /api/v1/me` that permanently erases the calling member's account. Confirmation is the member typing their own email address in the request body; a mismatch is rejected.
- Deletion is **irreversible and immediate** — no soft-delete, no grace period, no recovery window. The UI states this plainly before the member confirms.
- Artifacts outside Postgres are erased as part of the same operation, because the FK cascade cannot reach them:
  - **S3 objects**: the stored CV (`resumes/<id>`), every referral proof PDF (`referral-proof/<id>/<company>.pdf`), and the raw MIME of every hosted email (`emails.s3_key`). Keys are collected *before* the rows are deleted — afterwards they are unknowable.
  - **Google OAuth grant**: the encrypted Gmail refresh token is revoked at Google before the row is dropped, so freehire genuinely loses mailbox access rather than merely forgetting the token. This reuses the revoke path that `DELETE /me/gmail` already runs (`gmailsync.Connector.Revoke`).
- **BREAKING (session semantics)**: a JWT whose subject no longer exists is rejected. Today the stateless cookie parses fine after deletion, so a second device keeps a usable session until TTL expiry and its writes fail with FK violations (500) instead of 401. Sessions on every device stop working the moment the account is gone.
- Community content is preserved but de-authored: `threads.author_user_id` moves from `ON DELETE CASCADE` to `ON DELETE SET NULL` so a member's departure no longer takes other people's replies down with their thread. An authorless thread or reply renders as a distinct deleted-author marker — today a null handle renders as the AI persona, which would silently attribute a departed member's words to a bot.
- Account settings gain a "Delete account" surface listing exactly what will be erased, with the typed-email confirmation.

## Capabilities

### New Capabilities
- `account-deletion`: self-serve, irreversible erasure of a member's account — confirmation, transactional ordering across Postgres/S3/Google, what is erased, what deliberately survives (moderator audit trails, de-authored community content), and the account-settings surface that fronts it.

### Modified Capabilities
- `user-auth`: the stateless cookie session gains an existence check — a token for a deleted user is unauthenticated rather than merely stale.
- `community-threads`: authored content survives its author's deletion; the author identity of an authorless thread or reply is a deleted-member marker, distinct from the AI persona.
- `gmail-connection`: revocation at Google, today specified only for an explicit disconnect, is also required when the owning account is erased.

## Deploy note

**Apply `migrations/0041_threads_author_set_null.sql` by hand on prod BEFORE deploying
the binary.** `migrations/` runs through initdb only on a fresh volume. Unlike the usual
column-addition migrations, deploying first does not error — it silently keeps
`threads.author_user_id` on `ON DELETE CASCADE`, so the first account deletion destroys
other members' replies. That loss is unrecoverable, which is what makes the ordering
matter here more than usual.

## Impact

- **API**: new `DELETE /api/v1/me` (cookie-only, not reachable via API key). `internal/handler/handler.go` routing, new `internal/handler/me_delete.go`.
- **Auth**: `internal/auth/middleware.go` — `RequireAuth`/`RequireAuthOrKey` gain a user-existence lookup; `internal/auth` gets the resolver seam.
- **New package**: `internal/accountdelete` — orchestrates the ordered erasure (collect keys → revoke Google → delete rows → delete objects).
- **Migrations**: new migration flipping `threads_author_user_id_fkey` to `ON DELETE SET NULL`.
- **SQL**: `internal/db/queries/users.sql` (delete user, collect blob keys), `community.sql` (authorless thread reads currently `JOIN community_personas`, which would drop de-authored threads from listings — must become `LEFT JOIN`).
- **Storage/external**: `internal/blobstore` (bulk delete), `internal/gmail` (token revocation call to Google).
- **Web**: account settings page with the danger-zone surface; sign-out and redirect after a successful delete.
- **Docs**: `internal/auth/AGENTS.md`, `internal/handler/AGENTS.md`.
