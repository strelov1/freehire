## Why

freehire earns from job clicks only as a publisher in someone else's pay-per-click network,
which pays about $0.05 per valid click and keeps the rest of what the advertiser paid. The
catalogue already has the inventory, the job-seeker traffic, verified employer accounts and a
working Stripe integration. What is missing is a way for whoever wants more candidates on a
vacancy to pay freehire directly for clicks on it.

## What Changes

- New **sponsored campaigns**: anyone signed in can promote any open vacancy. A campaign has
  a cost per click (CPC, at or above a floor), a prepaid budget (minimum top-up USD 29) and an
  optional daily cap.
- **Prepaid budget through Stripe Checkout** in one-off payment mode (`mode=payment`). The
  existing client only does subscriptions. The webhook credits the campaign; there is no
  post-paid billing and no credit risk.
- **Moderation before going live**: because any payer may buy, a paid campaign waits in a
  moderator queue (`pending_review`) and runs only after approval. A rejected campaign is
  refunded through Stripe.
- **Three placements**, each labelled "Sponsored":
  1. Search results for a query: up to 2 sponsored jobs that match the same query and filters,
     on the first page only, highest CPC first.
  2. Saved-search alert emails: at most 1 sponsored job per email that matches the
     subscription's query.
  3. The company page: that company's sponsored jobs are pinned on top.
  - Not on the unfiltered `/jobs` feed, by product decision.
- **Server-side click charging**: sponsored placements link through a new redirect endpoint
  that validates the click, charges `min(CPC, remaining budget)` and returns 302 to the job's
  URL. A click is valid when it is not from a known bot, not a repeat from the same visitor on
  the same campaign within 24 h, and within the rate limits. When the budget or daily cap runs
  out, the campaign stops serving at once.
- **Buyer reporting**: impressions served, valid and invalid clicks, spend and remaining budget
  per campaign, on a new `/my/sponsored` page.
- Explicitly **not** in this change:
  - CPC auctions with quality scores
  - post-paid invoicing
  - refunds of unused budget (it stays on the campaign)
  - Stripe Tax / VAT handling
  - a sponsored slot on the unfiltered feed
  - selling clicks to other job boards through XML feeds
  - outreach to employers

## Capabilities

### New Capabilities

- `sponsored-jobs`: the campaign lifecycle, prepaid budget, moderation, the three placements,
  click validation and charging, and buyer reporting.

### Modified Capabilities

- `job-search`: the first page of a query may carry up to 2 labelled sponsored results in
  addition to the organic results.
- `email-notify`: a saved-search alert may carry one labelled sponsored job.
- `companies`: the company page pins that company's sponsored jobs.

## Impact

- **Database**:
  - New `sponsored_campaigns` table and `sponsored_clicks` ledger.
  - New `sponsored_impressions_daily` rollup.
  - Moderation status values on the campaign.
- **Search index**: new filterable `sponsored` (bool) and sortable `sponsored_cpc` fields on
  job documents, kept in sync through `search_outbox` whenever a campaign changes state.
- **Billing**: a one-off payment checkout next to the subscription checkout in
  `internal/identity/billing`. The webhook handles `checkout.session.completed` for campaign
  top-ups (idempotent by session id) and rejection refunds.
- **API** (typespec):
  - Campaign CRUD and top-up under `/api/v1/sponsored/*`.
  - A moderator queue.
  - A public redirect endpoint `GET /api/v1/sponsored/go/{token}`.
- **Web**:
  - A "Promote this job" entry on the job page.
  - The `/my/sponsored` dashboard.
  - A moderator queue page.
  - The "Sponsored" label in search results, on the company page and in the email template.
- **Risk**:
  - Click fraud by competitors draining a budget is mitigated by dedup, bot filtering, rate
    limits and the daily cap, but not eliminated.
  - Cached public search responses undercount impressions (counted when served, not when seen).
