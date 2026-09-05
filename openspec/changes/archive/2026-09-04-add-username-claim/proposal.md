## Why

Two features already invent their own version of "a short, unique, user-chosen name": the hosted mailbox (`<handle>@inbox.freehire.me`, derived silently from email) and the planned talent-network public profile (currently addressed by an opaque ID, per `talent-network-profile-visibility`). Neither is a name the user actually picked or can see reused elsewhere. Introducing `username` as a single account-level identifier now — before talent-network's profile URL ships — avoids two more copies of claim/uniqueness/reserved-word logic and gives users one name that follows them across surfaces.

## What Changes

- Add `users.username` (unique, GitHub-style format) and `users.username_updated_at` to the account model.
- New pure-function package `internal/identity/username`: format validation, a default-suggestion derivation from email, a reserved-word list, and a collision-suffix helper.
- New `internal/identity/accounts` service methods: `EnsureUsername` (lazy default allocation, first time one is actually needed) and `ClaimUsername` (explicit user-driven claim/change, rate-limited to once per 30 days, no auto-suffix on conflict — a taken name is just rejected).
- New endpoints: `GET /api/v1/username/check` (availability) and `PUT /api/v1/me/username` (claim/change).
- **BREAKING (internal only, no external API shape change):** `internal/application/mailbox` stops allocating its own handle. `mailbox.Handle`, `mailbox.Candidate`, and its `reservedHandles` list are removed from that package (their logic now lives in `internal/identity/username`); the mailbox address becomes `username + "@" + MAIL_DOMAIN`, resolved from `users.username` via `EnsureUsername` instead of a separately-allocated `mailboxes.address`.
- Migration backfills every existing `mailboxes.address` into `users.username`, so already-claimed inbox addresses do not change.

## Capabilities

### New Capabilities
- `identity/username-claim`: account-level username — format, reserved words, default suggestion, explicit claim with a 30-day change rate limit, availability check.

### Modified Capabilities
- `hosted-mailbox`: mailbox address allocation ("Claim a hosted mailbox address" requirement) changes from mailbox's own handle derivation/collision-suffix to reading the account's `username` (via `EnsureUsername`); claiming a mailbox no longer independently allocates a name, it adopts the caller's username.

## Impact

- **Schema:** new migration adding `users.username`, `users.username_updated_at`, plus a one-time backfill `UPDATE` from `mailboxes.address`.
- **Code:** new `internal/identity/username` package; `internal/identity/accounts` gains claim/ensure methods; `internal/application/mailbox` loses its own handle/candidate/reserved-word logic and instead depends on `internal/identity/accounts.EnsureUsername`.
- **API:** two new endpoints under `internal/api/handler` (username check + claim); no change to the existing mailbox status/claim/release endpoints' request/response shapes, only to what determines the returned address.
- **Out of scope for this change:** wiring `/u/<username>` into the talent-network public profile (left to a future change once `talent-network-profile-visibility` continues) and any `web/` UI for claiming a username.
