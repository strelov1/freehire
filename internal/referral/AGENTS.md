# Employee-referral conventions

## Scope
The employee-referral marketplace. Members offer to refer into a company (proof CV +
LinkedIn profile, manual moderation); seekers file a request that company's approved
referrers all see. Domain service and ports in referral.go, the sqlc adapter in
repository.go, delivery in pinger.go. HTTP lives in internal/handler/referrals.go: member
routes `/me/referrals/*` behind cookie-or-key auth, the moderation queue under
`/referrals/offers/*` behind the moderator gate (referrals.go:37-53).

## Always true
- **A request is company-scoped, not referrer-addressed.** Every approved referrer of the
  company sees it, and whichever one acts records the outcome — a second actor gets
  `ErrRequestNotOpen` (the status guard in SQL maps to it). This is what keeps the referrer
  anonymous to the seeker until they reach out over the contact the seeker provided.
- **IDs are random UUIDs because readers aren't always owners** — a countable id would make
  a single authorization slip enumerable (referral.go:101-102).
- **Withdrawal erases the proof CV BEFORE deleting the row, and refuses to delete when the
  object can't be erased** (`ErrProofStorageUnavailable` → 503, retry-safe). The offer row
  is the only thing that names the object — accountdelete finds a member's stored objects
  by reading these rows — so deleting the row first strands a CV in the bucket that nothing
  can reach again (referral.go:272-300). Same order and same reason as accountdelete. A nil
  blob store means no object storage, and that must NOT stop a withdrawal.
- **Pings are best-effort and deliberately minimal.** ChannelPinger emails every referrer
  (email is always present) and messages Telegram when linked; the notice is "you have a
  new referral request" plus a cabinet link, because the seeker's contact and CV live
  behind authorization in the cabinet — nothing leaks over the channel itself
  (pinger.go:25-29, 45-68). Per-channel failures are `errors.Join`'d, and the service logs
  and moves on: a delivery hiccup never fails the seeker's request (referral.go:421-434).
- **Sentinels carry their HTTP mapping in comments** (referral.go:50-96); the handler's
  error switch (referrals.go:139-167) is the only mapping. Validation sentinels fire before
  any DB touch; the DB-driven outcomes are mapped in repository.go — (user, company) unique
  violation → `ErrAlreadyOffered`, active-request partial-unique → `ErrAlreadyRequested`,
  company FK violation → `ErrCompanyNotFound`, no-row status-guard update →
  `ErrOfferNotPending` / `ErrRequestNotOpen`. Every write is a single statement, so no
  transaction wrapper is needed (repository.go:20-22).
- **A client-supplied built `cv_id` guarantees existence, not ownership.** A foreign cv_id
  is treated as an invalid choice rather than a distinct error, so the response never leaks
  that the CV exists (referral.go:326-338). CV access by a referrer goes through
  `AuthorizeCVAccess` — approved-referrer-of-the-company or nothing (referral.go:384-389).
- **LinkedIn URLs are shape-checked, not liveness-checked**: http(s) on linkedin.com with a
  `/in/<handle>` path, on both the offer and the request side (referral.go:444-461).

## How it works
Wiring is in handler.go:484-513: the SES client (reusing the notify worker's AWSRegion +
`NotifyEmailFrom`) and the Telegram bot are both optional — nil disables that channel, and
a referrer with no enabled channel still sees requests in-cabinet. Proof CVs go through
`cfg.Blob` (the S3_* blob store); `CabinetURL` is `FrontendOrigin` +
`/my/referrals?tab=incoming` (handler.go:510). `Config.DailyRequestCap` is a rolling-24h
per-seeker request limit (0 → `DefaultDailyRequestCap` = 10, referral.go:46-48); requests
are free but spam-resistant. `Config.Now` is the injectable clock for tests.
