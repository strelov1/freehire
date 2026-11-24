## Context

- `internal/api/handler/search.go` `SearchJobs` runs one Meilisearch query (`runJobSearch`) and
  returns public, cacheable results unless the answer depends on the caller.
- Saved-search alerts are matched in `internal/engage/notify/match.go` (`matchQuery`, freshest
  first) and delivered by `deliver.go` through `cmd/notify`.
- Jobs reach the index through the `search_outbox` queue.
- Stripe is called over raw REST in `internal/identity/billing/client.go`.
  `createCheckoutSession` hard-codes `mode=subscription`, and every entitlement is per user.
- `employer_vacancy_apply_clicks` (0305) counts daily apply clicks for employer-posted jobs
  only. Bot detection for views is a user-agent list in `internal/application/viewlog/bot.go`.
- WhatJobs pays freehire about $0.05 per valid click (Jul–Oct 2026 dashboard).
- Job pages get about 2 views each in 14 days (GA4, Nov 9–22). So a placement has to bring
  traffic to the job; the job page's own traffic is not enough.

## Goals / Non-Goals

**Goals:**
- Anyone pays freehire directly per valid click on a vacancy they promote.
- No credit risk.
- Spend that never exceeds the budget.
- Clicks the buyer can audit.

**Non-Goals:**
- Auction quality scores.
- Invoicing.
- Tax.
- Feed-based traffic resale to other job boards.
- Promotion on the unfiltered `/jobs` feed.

## Decisions

### D1. Prepaid budget, not post-paid billing

The buyer tops up through Stripe Checkout `mode=payment`, minimum USD 29, before anything
serves.
- Charging per click against prepaid money keeps all billing inside our ledger. There are no
  dunning, invoices or chargebacks for clicks already delivered.
- Alternative rejected: metered Stripe usage billing. It is post-paid, so a buyer can rack up
  clicks and not pay, and it needs usage-record reporting.

### D2. Any payer may buy, gated by moderation

The product decision is "anyone who pays", not only verified companies. So:
- A paid campaign enters `pending_review`.
- A moderator approves it into `active` or rejects it, and a rejected campaign is refunded in
  full through the Stripe refunds API.
- Campaigns from a buyer who holds an active `company_accounts` row for the job's company are
  still reviewed in v1. Auto-approval is a follow-up once fraud patterns are known.

### D3. Lifecycle

`draft → pending_payment → pending_review → active ⇄ paused`, then:
- `active → exhausted` when the budget runs out; a top-up returns it to `active`.
- `rejected` (refunded) or `ended` when the buyer stops it or the job closes.

A job closing (`closed_at` set) ends its campaigns; the unused budget stays recorded but does
not serve.

### D4. Index fields, kept in sync by `search_outbox`

- Job documents gain `sponsored` (bool, filterable) and `sponsored_cpc` (int cents, sortable).
- They are true only while some campaign on the job is `active` and has budget left for today.
- Every state change, and every charge that exhausts a budget or a daily cap, enqueues the job
  into `search_outbox`.
- The redirect endpoint reads campaign state from Postgres at click time, so a stale index
  cannot overspend (see D7).

### D5. Search placement

In `SearchJobs`, only when `offset == 0`:
- Run a second search with the same query and filter AND `sponsored = true`, sorted
  `sponsored_cpc:desc`, limit 2.
- Insert the hits at positions 1 and 6 of the organic page. Drop any duplicate of the same job
  from the organic list.
- Each sponsored view carries `sponsored: true` and a `sponsored_url` (redirect token), and
  keeps the organic `url` for crawlers.
- The second search runs in parallel with the organic one under the same timeout. If it fails,
  sponsored results are left out and the error is logged; the organic answer is never failed.
- Impressions are counted per campaign when served (`sponsored_impressions_daily`, batched
  upserts).

### D6. Email and company placements

- **Email:** when an alert email is built, run one extra search per subscription query with
  `sponsored = true`, limit 1, and render the job with a "Sponsored" label and a redirect link
  (`placement=email`).
- **Company page:** `/companies/[slug]` asks for `sponsored = true AND company_slug = X` and
  pins those jobs above the organic list (`placement=company`).

### D7. Click validation and charging

`GET /api/v1/sponsored/go/{token}`. The token is HMAC-signed and carries campaign id, job id,
placement and the issue time; it expires after 7 days for search and the company page, and 30
days for email.

In one transaction:
1. Verify the token. Load the campaign `FOR UPDATE`.
2. Always 302 to the job's URL, valid click or not, so a real seeker is never blocked.
3. Record a `sponsored_clicks` row with `valid` and a `reason`. Reasons:
   - `bot`: viewlog user-agent list, plus a missing user agent.
   - `duplicate`: same `visitor_hash = sha256(ip || ua || daily salt)` on the same campaign
     within 24 h.
   - `rate`: more than 20 clicks a minute from one IP.
   - `inactive`: campaign not active.
   - `budget`: no budget left.
4. For a valid click, charge `min(cpc, budget_left, daily_left)` cents. When that brings either
   limit to zero, mark the campaign `exhausted` or capped for today and enqueue `search_outbox`.

Postgres row locking makes overspend impossible. A stale index can only cause an extra
impression, never an extra charge.

### D8. Money

Integer cents, USD only.
- CPC floor: 10 cents, configurable as `SPONSORED_MIN_CPC_CENTS`.
- Minimum top-up: 2900 cents (`SPONSORED_MIN_TOPUP_CENTS`).
- Top-ups are idempotent on the Stripe checkout session id (unique column on
  `sponsored_topups`).

## Risks / Trade-offs

- **Competitor click fraud drains a budget.** Mitigation: visitor dedup, bot filtering, rate
  limits, an optional daily cap, and `invalid` clicks shown to the buyer. A follow-up may add
  IP reputation.
- **Low per-job traffic** (about 2 views in 14 days) may make campaigns underdeliver. The
  dashboard shows impressions so the buyer can see it.
- **Cached search responses** hide impressions. Accepted; impressions are a "served" count.
- **A refund on rejection** needs the PaymentIntent id; it is stored on top-up.

## Open questions

- Whether the CPC floor of 10 cents and the USD 29 minimum top-up are right. Both are config.
