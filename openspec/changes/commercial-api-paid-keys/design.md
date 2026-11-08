## Context

Motivation and measurements: see `proposal.md` and
`docs/superpowers/specs/2026-10-19-commercial-api-design.md`.

Current state that shapes the design:

- `internal/api/handler/public_read_limit.go` keys every public read limiter by IP and explains why
  the optional-auth gate was NOT moved in front of it: a database lookup ahead of throttling on the
  hottest reads, on a host whose traffic is mostly crawlers, and `OptionalAuth` answering 503 when
  a key lookup fails.
- `internal/identity/billing` resolves Pro and Ultra from one Stripe re-read
  (`entitlementFrom(sub, proPrices, ultraPrices)`) and stores both in one statement.
  `decideCheckoutTarget` picks the customer's best entitling subscription across Pro+Ultra prices
  and changes it IN PLACE when the requested price differs — so an API price requested by a Pro
  subscriber would today replace Pro.
- `embed`-scope keys are designed to be published inside third-party pages.
- nginx restores the client IP from `CF-Connecting-IP` for Cloudflare's ranges only, but the origin
  listens publicly on :443, so any header can be forged by a direct caller.

## Goals / Non-Goals

**Goals:** API access purchasable at two tiers; paid keys limited per account at their rate;
anonymous reads capped per minute and per day; verified bots exempt; a published data licence.

**Non-Goals:** feeds, change webhooks, change pull, snapshot files (later changes); manual grants
of API access; RevenueCat/app-store API sales; metering by record.

## Decisions

### D1. Product family decides checkout, not "any entitling subscription"

`decideCheckoutTarget` takes the price lists of the REQUESTED price's family: Pro/Ultra prices for a
Pro/Ultra request, API/Data prices for an API/Data request. Upgrade-in-place then only ever changes a
subscription of the same family. `Sells` admits all four lists. `moreThanOneEntitling` and the
billing overview stay Pro/Ultra-only.
*Alternative:* a second Stripe customer per family — rejected, two customers per person is the
exact duplication `CheckoutURL` was fixed to prevent.

### D2. Two plain timestamp columns

`users.api_until_stripe` and `users.api_data_until_stripe`, nullable, written together with
`pro_until_stripe`/`ultra_until_stripe` by `SetStripeEntitlement` (one statement, one re-read).
`entitlement` gains `API` and `Data`. No `_granted`/`_revenuecat` sources and no generated columns:
Stripe is the only seller (YAGNI; add a source column if manual grants are ever needed).
`billing-sync`'s due-soon query widens to the two new columns. Tier resolution is a pure function
`billing.APITierOf(apiUntil, dataUntil, now)` — billing owns the product.

### D3. Key resolution only when a Bearer key is presented, cached in process

The public-read gate inspects `Authorization` itself. No header → no lookup, so anonymous traffic
pays nothing new. With a `fhk_` Bearer token: hash, look up `(user_id, scope, api_until,
api_data_until)` in one query, and cache the RESULT (including "no tier" and "unknown key") in an
in-process TTL cache for 60 seconds keyed by the hash. A lookup error is not cached and the request
falls through to the anonymous rules (fail-open to the cheaper tier, never 503).
*Alternative:* move `OptionalAuth` in front — rejected for the reasons the existing comment gives;
it also bumps `last_used_at` per request, which is a write on the hottest path.
Consequence: a revoked key or lapsed tier keeps its paid rate for up to 60s; acceptable.
After review: the lookup also requires a minted key's shape, spends a per-IP lookup budget
(30 uncached lookups/min, checked without touching the response), runs under a 150ms timeout
and single-flight, and fake keys live in a bounded miss cache apart from real ones so they
cannot evict them.

### D4. One gate, budgets by caller class

A single `publicLimiter(budget)` middleware replaces `publicReadLimiter` and `agentSearchLimiter`:

1. trusted peer → pass (unchanged behaviour);
2. verified-bot header equals secret → pass;
3. paid full-scope key → `Allow("paid:<tier>:<user_id>", rate, 1m)`;
4. otherwise → `anon:<ip>` 120/1m, and only if that admits, `anonday:<ip>` 2000/24h. GCRA
   charges only what it admits, so a minute refusal costs no day; the admitted response reports
   the minute's headers (`ratelimit.Check` + `ratelimit.Apply`).

`/mcp` and `/mcp/account` mount `chatAgentLimiter`, which keeps the agent-search per-minute budget
under strict: OpenAI's shared egress would turn a per-IP daily cap into a cap on the channel.
Under strict the OJCP manifest declares no rate, since 2,000/day has no honest per-second figure.

Anonymous budgets are shared by read and agent-search routes on purpose: a mirror switching routes
must not double its allowance. Paid budgets are per account (all its keys), not per key. The
budget headers reflect the check that decided (the day check when it refuses). The 429 message
names `/pricing#api`.
The per-route trusted-peer and fail-open semantics come from `ratelimit` unchanged; the gate calls
the exported `Throttler` directly for the two-step anonymous check.

### D5. Verified bot = Cloudflare Transform Rule + secret

A Request Header Transform Rule sets `X-FH-Verified-Bot: <secret>` when `cf.client.bot`. Go compares
with `subtle.ConstantTimeCompare` against `VERIFIED_BOT_SECRET`; unset disables the exemption.
Transform Rules are a separate quota from the 5 custom WAF rules already in use.

### D6. Rollout order

One switch, `PUBLIC_ANON_LIMITS`: unset keeps TODAY's anonymous budgets exactly (600/min reads and
300/min agent search, each its own IP key, no daily cap); `strict` applies the shared 120/min +
2,000/day. Paid-key and verified-bot handling are active either way. Order: deploy → create Stripe
prices → set price env → check `/pricing` → add the CF rule and secret → set `strict`. Rollback is
unsetting it.
*Alternative:* hard-coded new numbers — rejected, it would throttle mirrors before they can buy.

## Risks / Trade-offs

- [Mirrors behind many IPs evade the daily cap] → accepted; the cap's job is to make the cheapest
  path paying, not to be airtight.
- [ai-job-search users behind one NAT hit 2,000/day] → measured max for a skill IP is 3,988/day and
  rare; 429 carries Retry-After, and the skill authors can be offered keys.
- [Cached tier lags revocation by 60s] → bounded and documented.
- [Stripe event for an API sub arrives before the price env is set] → it entitles nobody (unknown
  price), and `billing-sync` reconciles once the env is set.
- [The repo's guard tests assert IP-only keys on public reads] → rewritten to assert the new
  classes, including that a full-scope key of an UNPAID account stays IP-keyed.

## Migration Plan

1. Migration adds the two nullable columns (no backfill, no lock beyond ADD COLUMN).
2. Deploy code (limits unchanged by default).
3. Ops in `freehire-ops`: Stripe product + prices, env, CF Transform Rule, secret.
4. Flip anonymous limits by env; watch 429 rate and `ratelimit: refused` log lines for 24h.
Rollback: restore the env numbers; the columns are inert.

## Open Questions

- **Discounts on the API product.** Checkout applies promo and invite discounts to any sold
  price, and a collected API invoice would settle a referral reward. Undecided.
- **Visitors behind one address.** The SPA calls the public API from the browser, so under
  strict an office or campus NAT shares 2,000/day. Measure page-originated API calls per IP
  from the access log before setting strict.
- API-only subscribers have no in-app billing section (overview stays Pro/Ultra); they manage
  the subscription through the provider's portal.
- Adzuna and other aggregator sources may forbid sublicensing their postings; checked before the
  licence text ships (task 6.1), and if needed those sources are named as excluded from the licence.
