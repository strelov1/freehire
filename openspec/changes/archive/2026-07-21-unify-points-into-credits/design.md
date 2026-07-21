## Context

Two "points" quantities coexist. `users.points` (added in `0025_link_contributions`) is a
per-user counter incremented `+1` per accepted board contribution and shown on
`/my/contributions`. The AI-credits ledger (`0031_credits`, `0032_credit_reward_kind`) is the
real metered-features economy: `credit_ledger` is append-only (kinds `grant`/`debit`/`reward`/
`purchase`), `credit_balances` is the materialized per-user cache. A contribution already awards
`+5` credits via `credits.Reward()` (best-effort, idempotent by contribution id), so
`users.points` is redundant and confusing.

The `internal/credits` package, the ledger schema, and the reward/debit logic are unchanged by
this work. We remove the legacy counter, restate the contribution UI in credits, consolidate the
balance widget onto one page, and add a history read path.

Ledger `ref` semantics (all numeric strings; `grant` has NULL ref):
- `debit` + `feature=match` → job id (`match_analysis.go` `debitMatch`)
- `debit` + `feature=tailor` → tailored-CV id (`cv_tailor.go`)
- `reward` → contribution id

## Goals / Non-Goals

**Goals:**
- Remove `users.points` and every read/write/display of it; contribution reward is solely AI credits.
- New `/my/credits` page: current balance + human-labelled transaction history.
- New `GET /api/v1/me/credits/history` endpoint over `credit_ledger`, newest first.
- Remove the inline `CreditsBalance` widget from Activity → Matches and Profile.
- Add a "Credits" account-nav item.

**Non-Goals:**
- No change to credits economics, `credit_ledger`/`credit_balances` schema, or `Reward`/`Debit`.
- No cursor/infinite-scroll pagination — a bounded recent-N list is sufficient for MVP.
- No revocation of rewards for later-rejected contributions (unchanged existing behaviour).

## Decisions

**1. Drop `users.points` via a new migration.** A forward migration `ALTER TABLE users DROP
COLUMN points`. Per project convention, on prod this is run manually (`SET ROLE hire`) before the
code that stops reading it deploys. The `IncrementUserPoints` query and its call in
`contribution/repository.go` are deleted; `Record` no longer touches users. Alternative (leave the
column dormant) rejected — the whole point is to remove the confusing concept from the schema too.

**2. Resolve history labels in Go, not in one big SQL join.** The new sqlc query
`ListCreditLedger` returns raw ledger rows for a user (newest first, `LIMIT`). The handler then
batch-resolves refs: collect `match` refs → one `jobs WHERE id = ANY($1)` lookup for title/slug;
collect `tailor` refs → one lookup for the tailored CV; grants and rewards need no lookup. Each
entry is projected to a DTO `{kind, feature, delta, label, subtitle, created_at}`. Rationale:
keeps the SQL trivial and index-friendly (`credit_ledger_user_id_created_at_idx`), makes the
missing-subject fallback explicit, and is straightforward to unit-test. A cast-heavy multi-LEFT-
JOIN over a text `ref` column was considered and rejected as fragile and hard to test.

**3. Labels are computed server-side.** The endpoint returns display-ready `label`/`subtitle`
so the frontend stays dumb: e.g. `Monthly grant` (+N), `Board contribution` (+N),
`Match analysis · <job title>` (−1), `CV tailoring` (−3). A debit whose subject was deleted
falls back to the bare feature label. This centralizes the vocabulary and avoids duplicating the
ref-resolution logic in TypeScript.

**4. Bounded history, no pagination for MVP.** Return the most recent N (e.g. 100) entries. The
ledger for a single free-tier user is small; a cursor API can be added later behind the same
endpoint if needed. The seam (a `limit`/`before` query param) is noted, not built.

**5. Balance page reuses the existing balance endpoint.** `/my/credits` reads `GET
/api/v1/me/credits` for the headline number and `GET /api/v1/me/credits/history` for the list.
No new balance computation.

## Risks / Trade-offs

- **Dropping a column on prod** → Follow the established manual-migration runbook (`SET ROLE hire`
  before deploy); code that no longer selects `points` is compatible with the column still present,
  so ordering (migrate-then-deploy) is not tight, but the users query must drop `points` in the
  same release to match the regenerated model.
- **Ref → title resolution is best-effort** → A deleted job/CV yields a generic label, never an
  error; the amount always renders. Covered by a fallback scenario/test.
- **`ListCreditLedger` returns non-user-facing kinds (`purchase`)** → The label mapper has a
  default branch so an unrecognized kind still renders with its delta, future-proofing the
  currently-unused `purchase` kind.

## Migration Plan

1. Ship migration `00XX_drop_users_points.sql` (`ALTER TABLE users DROP COLUMN points`).
2. On prod: `SET ROLE hire; ALTER TABLE users DROP COLUMN points;` before deploying the new image.
3. Deploy backend (users query without `points`, new history endpoint) + frontend (no widget,
   new page, credits copy) together.
4. Rollback: re-add the column (`ADD COLUMN points integer DEFAULT 0 NOT NULL`) if needed; the
   counter is not repopulated (it is dead data), so rollback is only to satisfy an older image.

## Open Questions

- None blocking. History depth default (100) is a tunable, not a design fork.
