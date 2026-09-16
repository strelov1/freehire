## Why

Payment for the AI features just went live and the first real subscriber has already paid
(2026-09-15), reached by hand. Nothing in the product today tells a new subscriber their
payment landed beyond Stripe's own receipt, and nothing in the header shows a paying
account that it is paying — both things most subscription products do on day one, and both
currently require a person to notice and act by hand.

## What Changes

- Add a new periodic worker (`internal/engage`, alongside `discord-sync`/`billing-sync`'s
  pattern) that detects an account's first-ever transition into a paying tier (pro or
  ultra) and sends it a one-time, personal, founder-voiced welcome email — styled after
  `internal/engage/onboarding`'s mailer (first-person prose, Reply-To a human inbox), not
  the machine-toned digest mailer.
- Add `users.pro_welcome_sent_at` (new migration) so the send is idempotent and a renewal,
  a plan change, or a reconciler replay never re-sends it.
- Add a `tier` field to the signed-in user's public shape (`GET /api/v1/auth/me`), derived
  via the existing `plan.TierOf`, so the frontend learns the caller's tier for free on the
  request that already powers every page.
- Show a small "PRO"/"ULTRA" badge on the profile icon in the site header (desktop) when
  the signed-in account's tier is not free.

## Capabilities

### New Capabilities
- `pro-welcome-email`: detecting a first-ever transition into a paying tier and sending a
  one-time, personal welcome email for it.

### Modified Capabilities
- `header-navigation`: the header's profile icon (desktop) gains a tier badge for a paying
  account.

## Impact

- **New migration**: `users.pro_welcome_sent_at` (nullable timestamptz).
- **New package + binary**: an `internal/engage` package and a `cmd/<worker>` entry, plus a
  new systemd timer to record in `deploy/` (not installed on host2 by this change — that is
  a manual follow-up per `deploy/AGENTS.md`'s "nothing here deploys itself"). It lives in
  `engage`, not `identity/billing`, because `identity` must not import `engage` — the same
  layering rule `deploy/AGENTS.md` documents for why `discord-sync` is a separate worker
  from `billing-sync` rather than a pass inside it.
- **`internal/api/handler/auth.go`**: `userResponse`/`toUserResponse` gain a `tier` field,
  computed via `internal/ai/plan.TierOf`.
- **Frontend**: `web/src/lib/types.ts` (`User.tier`), `web/src/lib/components/HeaderMenu.svelte`
  (badge on the desktop profile icon).
- **No changes to `internal/identity/billing`** — it stays exactly what
  `internal/identity/billing/AGENTS.md` describes: a provider-generic sync that derives the
  source column whole. The new worker reads the *result* of that sync, never a hook inside it.
