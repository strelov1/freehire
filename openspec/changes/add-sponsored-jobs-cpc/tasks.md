## 1. Database schema

- [x] 1.1 Migration `sponsored_campaigns`:
      - id, `job_id` FK, `owner_user_id` FK, `status` CHECK, `cpc_cents` >= 1
      - `daily_cap_cents` NULL, `budget_cents`, `spent_cents`, CHECK spent <= budget
      - `reviewed_by`, `reviewed_at`, `reject_reason`, timestamps
      - index on (status, job_id)
- [x] 1.2 Migration `sponsored_topups`:
      - campaign_id, `stripe_session_id` UNIQUE, `payment_intent_id`, `amount_cents`
      - `refunded_at`, created_at
- [x] 1.3 Migration `sponsored_clicks`:
      - campaign_id, ts, placement, `visitor_hash`, `valid`, `reason`, `cost_cents`
      - index on (campaign_id, visitor_hash, ts)
- [x] 1.4 Migration `sponsored_daily(campaign_id, day, impressions, valid_clicks,
      invalid_clicks, spent_cents)`
- [x] 1.5 sqlc queries:
      - create, get-for-update, state transitions
      - charge (an UPDATE that checks budget and daily cap)
      - daily rollup upserts, owner listing, moderation queue

## 2. Campaign domain (`internal/engage/sponsored`)

- [x] 2.1 Lifecycle transitions with tests for every allowed and refused transition
      (TDD first)
- [x] 2.2 `Charge(campaign, visitor, now)`:
      - validity reasons, `min(cpc, budget_left, daily_left)`
      - a concurrency test proving no overspend
- [ ] 2.3 Close hook: closing a job ends its campaigns
- [x] 2.4 Enqueue `search_outbox` on every state change and on budget or cap exhaustion

## 3. Billing

- [ ] 3.1 `client.createPaymentCheckoutSession` (`mode=payment`, metadata
      campaign_id/user_id) with a client test
- [ ] 3.2 Webhook: `checkout.session.completed` with campaign metadata credits the budget
      idempotently by session id. Draft becomes `pending_review` on the first top-up;
      exhausted becomes `active` on a later one.
- [ ] 3.3 Refund call (`POST /v1/refunds`, payment_intent) used on rejection, with a test

## 4. Search index and placements

- [ ] 4.1 `JobDocument` gains `sponsored` and `sponsored_cpc`; index settings make them
      filterable and sortable; the outbox builder fills them from active campaigns
- [ ] 4.2 `SearchJobs`:
      - a parallel sponsored lookup on offset 0
      - insert at positions 1 and 6, dedupe, add `sponsored` and `sponsored_url` to the view
      - degrade on error
      - tests
- [ ] 4.3 Impression counting (batched upsert into `sponsored_daily`)
- [ ] 4.4 Alert email: one sponsored job per subscription query, with label and redirect link;
      template test
- [ ] 4.5 Company page: pinned sponsored jobs

## 5. Redirect endpoint

- [ ] 5.1 HMAC token issue and verify, with expiry per placement (key from config)
- [ ] 5.2 `GET /api/v1/sponsored/go/{token}`:
      - always 302 to the job URL
      - bot list from viewlog, visitor hash with a daily salt, per-IP rate limit
      - `Charge` in one transaction
      - tests for the reasons and the redirect

## 6. API and contract

- [ ] 6.1 typespec `sponsored.tsp`:
      - create, list mine, get, pause/resume/end, start a top-up (returns a checkout URL)
      - moderation list/approve/reject
      - regenerate ogen and the web client
- [ ] 6.2 Handlers with authorization: owner-only campaign access; moderator-only queue

## 7. Web

- [ ] 7.1 "Promote this job" on the job page: form for CPC, budget and daily cap, then
      redirect to checkout
- [ ] 7.2 `/my/sponsored`: campaigns, status, per-day impressions, clicks, invalid clicks by
      reason, spend, top-up
- [ ] 7.3 Moderator queue page
- [ ] 7.4 "Sponsored" label in search results and on the company page (accessible text, not
      colour alone)

## 8. Verification

- [ ] 8.1 Integration test end to end: create, top-up webhook, approve, search shows it,
      redirect charges, exhaust, search hides it
- [ ] 8.2 Simplify pass, code review, fix findings
- [ ] 8.3 Stripe test-mode run on staging, or locally with the Stripe CLI webhook forward
