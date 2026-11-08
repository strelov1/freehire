## Why

The public read API is free and limited only per IP (600/min reads, 300/min agent search). Measured
on production for 2026-10-19, 34 of 24,859 API-calling IPs made more than 2,000 requests that day,
and they are almost all catalogue mirrors naming themselves in the User-Agent
(`aarivi-catalog-sync` 206k requests, `VivekJobBoard` 48.6k, `KerrasTechJobRadar` 41.8k,
`freehire-dump` 22.4k, …). They take the most capacity and pay nothing, while competitors charge
$1k–3k/month for the same catalogue. Nothing lets them pay us either. This change makes access
purchasable and makes the free tier a free tier. It is step 1 of
`docs/superpowers/specs/2026-10-19-commercial-api-design.md`; feeds, change webhooks and snapshot
files are later changes.

## What Changes

- A separate Stripe product "API" with two prices: **API ($29/mo)** and **Data ($99/mo)**. Buying
  it never modifies an existing Pro/Ultra subscription, and buying Pro never modifies an API one.
- New account columns recording how far each API tier is paid for, written by the existing
  webhook and `billing-sync` reconcile.
- Public read routes recognise a `full`-scope API key whose owner holds a live API tier and limit
  it **per account** at the tier's rate (1,200/min API, 6,000/min Data) instead of per IP.
- **BREAKING (free callers):** anonymous public reads drop from 600/min (reads) and 300/min (agent
  search) to **120/min per IP plus 2,000/day per IP**, shared across the read and agent-search
  routes. The daily cap is what stops catalogue mirrors; the per-minute cap stays loose enough for
  the ai-job-search skill's 200–300/min bursts to degrade rather than break.
- Requests Cloudflare has verified as a known search/AI bot are exempt from the anonymous limits,
  recognised by a secret header set by a Cloudflare Transform Rule.
- A 429 from a public read names the paid tier.
- Terms gain a data licence for paid keys; the OpenAPI document gains `bearerAuth` and the new
  limits; `/pricing` offers the two API plans.

## Capabilities

### New Capabilities

- `commercial-api`: the API subscription product, its entitlement, tier-aware and anonymous limits
  on public reads, the verified-bot exemption, and the data licence.

### Modified Capabilities
<!-- none: api-rate-limiting's requirements (shared backend, namespaced keys, fail-open, headers)
     still hold; the budgets themselves were never specified there -->

## Impact

- `internal/identity/billing` (config, entitlement, stripe store, checkout target), `cmd/billing-sync`.
- New migration: API entitlement columns on `users`; new queries for key → tier resolution.
- `internal/api/handler/public_read_limit.go` and every route mounting it; `internal/api/ratelimit`
  untouched in contract.
- `web/static/openapi.yaml`, `web/src/routes/terms`, `web/src/routes/pricing`, `web/src/routes/my/api-keys`.
- Ops (freehire-ops, by hand): Stripe product + prices, `STRIPE_API_PRICE_IDS`,
  `STRIPE_API_DATA_PRICE_IDS`, `VERIFIED_BOT_SECRET`, a Cloudflare Transform Rule.
- Every anonymous heavy caller sees 429s sooner; the rollout lowers the anonymous limits only after
  the paid plans are purchasable.
