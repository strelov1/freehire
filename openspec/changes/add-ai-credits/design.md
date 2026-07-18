## Context

Two user-facing AI features cost budget: `job-fit-analysis` (3 LLM calls per
fresh analysis, paid by freehire) and `tailor-workspace` (0 backend LLM calls —
the cost is borne by the external roy agent, but the feature is premium). Today
only match is metered, via `enforceFitQuota` in `internal/handler/job_fit.go`,
which counts distinct `user_job_analysis` rows in a rolling 30-day window — an
implicit meter with no ledger, no audit trail, and no path to paid top-ups.

We are building a real, unified points ledger now (issued free), designed so a
future paid `kind = 'purchase'` grant plugs in without reworking the debit path.
The existing pre-LLM gate location is the natural seam.

## Goals / Non-Goals

**Goals:**
- One auditable points balance per user, drawn by both match and tailor.
- Monthly grant (default 20), use-it-or-lose-it, lazy reset — no cron.
- Per-action costs (match 1, tailor 3), configurable via env.
- Atomic, idempotent debit that never oversells under concurrency.
- Replace `enforceFitQuota` with a points pre-check + on-success debit.
- Ledger shaped to accept a future purchase grant with no migration.

**Non-Goals:**
- Stripe / paid purchase flow (only the ledger shape must be forward-compatible).
- Metering `resume-extract` (background, not user-invoked).
- Unused-credit rollover between periods.
- Separate per-feature quotas (a single balance with weighted costs replaces
  the earlier idea).

## Decisions

### Two tables: append-only ledger + materialized balance cache

`credit_ledger` is the source of truth (immutable rows); `credit_balances` is a
one-row-per-user cache read on the hot debit path so the gate never runs a
`SUM()` over history.

```sql
CREATE TABLE credit_ledger (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  period     text   NOT NULL,               -- 'YYYY-MM' (UTC)
  kind       text   NOT NULL CHECK (kind IN ('grant','debit')),  -- 'purchase' later
  feature    text   CHECK (feature IN ('match','tailor')),       -- NULL for grants
  delta      integer NOT NULL,              -- +grant / -cost
  ref        text,                          -- job_id (match) / cv_id (tailor)
  created_at timestamptz NOT NULL DEFAULT now()
);
-- idempotency: one debit per (user, feature, ref)
CREATE UNIQUE INDEX credit_ledger_debit_ref_uniq
  ON credit_ledger (user_id, feature, ref) WHERE kind = 'debit';
-- one monthly grant per (user, period)
CREATE UNIQUE INDEX credit_ledger_grant_period_uniq
  ON credit_ledger (user_id, period) WHERE kind = 'grant';

CREATE TABLE credit_balances (
  user_id    bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  period     text   NOT NULL,
  remaining  integer NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
```

The `kind` CHECK admits `'purchase'` later; the grant-uniqueness index is scoped
to `kind = 'grant'` so purchase grants are not constrained to one-per-period.

*Alternative considered:* balance-from-ledger-only (no cache). Rejected — the
debit gate is on the request hot path and a per-request aggregate over an
ever-growing ledger is needless cost; the cache is trivially reconstructable.

### `internal/credits` package mirroring `internal/jobfit`

A small `Store` holding `*db.Queries` (and a `*pgxpool.Pool` for the debit
transaction), exposing:
- `Balance(ctx, userID) (Balance, error)` — read `{Remaining, ResetsAt}`,
  applying a lazy reset in-memory for display (does not need to write).
- `Debit(ctx, userID, feature, ref) (Balance, error)` — atomic; returns the
  post-debit balance or a typed `ErrInsufficient` the handler maps to 402.

Costs and the monthly grant come from `internal/config` (`CREDITS_MONTHLY_GRANT`,
`CREDITS_COST_MATCH`, `CREDITS_COST_TAILOR`) with the stated defaults.

### Atomic debit: single transaction, row lock, lazy reset inside

`Debit` runs one transaction:
1. `SELECT ... FROM credit_balances WHERE user_id=$1 FOR UPDATE` (serializes per
   user; `NULL` row → treat as fresh).
2. If the row's `period` ≠ current period (or no row): the period rolled over —
   set `remaining = monthlyGrant` and append the `grant` ledger row for the new
   period (`ON CONFLICT DO NOTHING` on the grant-period index guards double
   grant across races).
3. If a `debit` row already exists for `(user, feature, ref)` → idempotent no-op,
   return current balance (recompute/resume is free).
4. If `remaining < cost` → roll back / return `ErrInsufficient`.
5. Else decrement `remaining`, `upsert credit_balances`, append the `debit`
   ledger row.

Row-level `FOR UPDATE` gives the no-oversell guarantee without a global lock.

*Alternative considered:* optimistic `UPDATE ... WHERE remaining >= cost` without
an explicit lock. Rejected — the lazy-reset + idempotency-check + two writes are
cleaner and race-safe under a single `FOR UPDATE` than as a chain of conditional
statements.

### Debit timing: pre-check before LLM, commit debit only on success

Match keeps the existing pattern: a cheap pre-check (`Balance` ≥ match cost, and
whether this `(user, job)` was already debited) BEFORE the prompt-chain, so we
never burn LLM tokens for a user who can't pay; the actual `Debit` fires only
after the analysis is persisted. Idempotency by `job_id` means a failed run
(no persist) leaves the balance untouched and a later retry debits once.

Tailor has no LLM to protect, so `TailorCV` calls `Debit(..., "tailor", cvID)`
transactionally as part of creating the tailored CV; on `ErrInsufficient` it
returns 402 and creates nothing.

### Response contract

Insufficient balance → `402` `{error, remaining, resets_at}`. Match's read
endpoint swaps its `quota {used,limit,remaining}` object for
`credits {remaining, resets_at}`. `resets_at` is the first day of next month UTC.

## Risks / Trade-offs

- **Behavior change 429 → 402 for match** → the frontend match page must handle
  402; ship the FE change in the same PR. Documented as internal-breaking.
- **Lazy reset only fires on access** → a user who never acts sees a stale
  `credit_balances` row, which is harmless (balance is recomputed/reset on next
  read or debit). Display reads apply the reset in-memory so the shown number is
  always correct.
- **Dropping the 30-day rolling window for a calendar month** → a user could get
  a fresh grant sooner (month boundary) than under the old rolling window; this
  is the intended, simpler model and acceptable at free tier.
- **Grant/debit race on first action** → both the grant-period unique index and
  `FOR UPDATE` serialization prevent double grants and oversell.

## Migration Plan

- Add `migrations/NNNN_credits.sql` (both tables + indexes). Migrations apply via
  Postgres initdb on fresh volumes; on prod apply the migration manually per the
  project's manual-migration ownership procedure (SET ROLE / ALTER OWNER).
- No backfill: existing users simply receive their first grant lazily on next
  metered action. Historical `user_job_analysis` rows are left as-is; the new
  meter is independent of them.
- Rollback: revert the handler wiring (restore `enforceFitQuota`); the two tables
  can be dropped independently since nothing else references them.

## Open Questions

- None blocking. Exact default grant/cost numbers are env-configurable and can be
  tuned post-launch without code changes.
