## Why

The `/open` transparency page shows catalogue and member metrics but nothing about
what people *do* on freehire. The `user_jobs` table already records every save,
application, and view, so surfacing those aggregate counts is a cheap, honest
engagement signal — no new instrumentation, no PII.

## What Changes

- Add a public **`GET /api/v1/stats/engagement`** endpoint returning aggregate
  interaction counts — jobs saved, applications marked, jobs viewed — computed on
  the fly from `user_jobs` (aggregate-only, no user identifier).
- Add an **Engagement** stat-strip section to the `/open` page, linking the figures
  to the endpoint (same API-first idiom as the rest of the page).

Out of scope (deferred by decision): counting search queries — that needs new
write-path instrumentation on the hot `/jobs/search` endpoint and is tracked
separately.

## Capabilities

### New Capabilities
- `engagement-stats`: a public `GET /api/v1/stats/engagement` endpoint returning
  aggregate `user_jobs` interaction counts (saved / applied / viewed), computed
  directly from the table with no rollup and no personal fields.

### Modified Capabilities
<!-- None. The /open page consumes the new endpoint read-only; its existing
     requirements are unchanged (a new section is additive). -->

## Impact

- **Backend**: new `GetEngagementStats` query in `internal/db/queries/stats.sql`
  (→ `make sqlc`), an `EngagementStats` handler (a `UserGrowth` sibling), and a
  public route in `handler.Register`.
- **Frontend**: `/open/+page.server.ts` fan-out gains an `engagement` leg
  (best-effort); a new Engagement stat-strip section; `api.engagementStats()` +
  an `EngagementStats` type.
- **No migration, no new env, no worker.**
