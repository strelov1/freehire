## Why

The user-facing AI features (résumé match and CV tailoring) each cost real LLM
budget, yet only match has a meter today — an implicit, hard-to-audit
distinct-job quota computed by counting cache rows. Tailoring is unmetered. To
control LLM spend fairly and lay a foundation for future monetization (candidate
SaaS, ladder step 3), we need a single, auditable per-user points balance that
both features draw from, with a real ledger that a paid top-up can later plug
into.

## What Changes

- Introduce a **unified points balance** per user, replenished by a **monthly
  grant** (default 20 points, configurable via env; use-it-or-lose-it; reset
  lazily on first access in a new period — no cron).
- Meter both AI actions against that balance with **per-action costs** (default
  match = 1, tailor = 3; configurable via env).
- Persist an **append-only ledger** (`credit_ledger`) as the source of truth and
  a **materialized balance cache** (`credit_balances`) for the hot debit path.
- **Match:** debit on successful fit-analysis persist, idempotent by `job_id`;
  a recompute of an already-analyzed job stays free. **BREAKING (internal):**
  replaces the existing `enforceFitQuota` distinct-job meter (10 / 30 days).
- **Tailor:** debit on new tailored-CV creation (`POST /me/cvs/tailor`),
  idempotent by `cv_id`. No backend LLM runs here — the debit prices the feature.
- **Insufficient points** returns **HTTP 402** with `{error, remaining,
  resets_at}` (match previously returned 429). The current remaining balance and
  reset date are surfaced to the client on `GET /jobs/:slug/fit` and the tailor
  responses.

## Capabilities

### New Capabilities
- `ai-credits`: the per-user points balance, monthly grant + lazy reset,
  append-only ledger, materialized balance cache, atomic idempotent debit, and
  the balance/insufficient-points contract exposed to callers.

### Modified Capabilities
- `job-fit-analysis`: the match quota requirement changes — the distinct-job
  (10 / 30-day) meter is replaced by a points debit (cost = match) enforced via
  the `ai-credits` capability; over-limit response changes from 429 to 402.
- `tailor-workspace`: tailored-CV creation now debits points (cost = tailor)
  before minting the tailoring session; over-limit response is 402.

## Impact

- **New:** `migrations/NNNN_credits.sql` (two tables); `internal/credits`
  package; sqlc queries in `internal/db/queries/credits.sql`; config vars
  (`CREDITS_MONTHLY_GRANT`, `CREDITS_COST_MATCH`, `CREDITS_COST_TAILOR`).
- **Modified:** `internal/handler/job_fit.go` + `job_fit_stream.go` (replace
  `enforceFitQuota` with a points pre-check + on-success debit; response shape);
  `internal/handler/cv_tailor.go` (`TailorCV` debit); handler wiring in
  `internal/handler/handler.go` to inject the credits store.
- **Frontend:** match page and tailor surface read the new
  `{remaining, resets_at}` fields and render an out-of-credits (402) state.
- **Out of scope:** Stripe / paid purchase (ledger is designed to later accept a
  `kind = 'purchase'` grant), resume-extract metering, unused-credit rollover,
  and separate per-feature quotas.
