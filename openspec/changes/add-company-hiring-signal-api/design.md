## Context

The prior change shipped `insights_company_stats` (per-day velocity + running open) and the `cmd/rollup-company` worker, deployed on prod. This change adds the first read surface — a public leaderboard — following two existing patterns:

- **`market-insights`** (`internal/handler/insights.go`): public `/insights/*` endpoints, `{"data":[...],"meta":{...}}` envelope, `parseInsightsSort`/`parseInsightsLimit` helpers, ranked SQL `ORDER BY (CASE WHEN sort='growth' …) DESC LIMIT`.
- **`insights_role_stats`** (`migrations/0022`): a precomputed per-entity scalar (`open_count`, `open_count_prev`) that makes ranked reads a trivial indexed sort — the model for how a leaderboard should be backed.

## Goals / Non-Goals

**Goals:**
- Public `GET /api/v1/insights/companies` leaderboard (ramping / freezing / size).
- Back it with a precomputed scalar so reads never aggregate ~155k companies per request.
- A `min_open` floor so ingest-artifact spikes don't dominate the default view.

**Non-Goals:**
- Single-company detail/time-series endpoint (later slice).
- Any auth/API-key gating (public, like the rest of `/insights/*`).
- Smoothing/deduping ingest churn beyond the blunt `min_open` floor.

## Decisions

### Precompute a per-company scalar `insights_company_growth`

`insights_company_growth(company_slug text PRIMARY KEY, open_count int, open_count_prev int)`, rebuilt from `jobs` (not from `insights_company_stats`) with the same `count(*) FILTER (…)` idiom as `insights_role_stats`: `open_count` = open canonical jobs now, `open_count_prev` = open as of `now − 30d`. Ranked read = `ORDER BY (open_count − open_count_prev)` or `open_count`, `LIMIT`.

**Alternatives considered:**
- *Read-time `DISTINCT ON (company_slug)` over `insights_company_stats` per request*: a full sort/dedup over ~681k rows / ~155k groups on every call. Rejected — that is exactly the read-time aggregation `insights_role_stats` exists to avoid.
- *Derive the scalar from `insights_company_stats` (carry-forward)*: more complex than counting `jobs` directly, and couples the two tables. Rejected — counting `jobs` is one simple aggregate, same as the sibling rollups.

### Rebuild inside the existing rollup transaction

`cmd/rollup-company` already opens one transaction with `SET LOCAL work_mem` for `insights_company_stats`. The new table's delete+rebuild joins that same transaction (before commit), so both are swapped atomically and a single cron run keeps them consistent. The 30-day `prev_ts` is computed in Go and passed to the SQL, exactly like `cmd/rollup-stats` does (`growthWindowDays = 30`).

### Read query joins `companies` for the display name

`ListInsightsCompanies` LEFT JOINs `companies` on `slug = company_slug` for `company_name`, falling back to the slug when no company row exists. The join touches only the `LIMIT`ed result rows.

### Sort + params

New `parseCompaniesSort` accepting `growth` (default), `-growth`, `open` — the existing `parseInsightsSort` only knows `growth`/`open` and has no descending-growth (freezing) case, so a dedicated parser is clearer than overloading it. `min_open` parsed with a small positive default and a floor of 0; `limit` reuses `parseInsightsLimit` (shared cap).

## Risks / Trade-offs

- **Ingest-artifact spikes** (a board fully appearing/disappearing shows as a huge ramp/freeze) → mitigated bluntly by the `min_open` default; a real smoothing/attribution layer is explicitly a later concern (noted in the shipped rollup's memory).
- **Scalar staleness between rollup runs** (daily cron) → acceptable; the signal moves slowly and this matches how the other `insights_*` reads work.
- **155k-row sort per request for growth sort** (not index-friendly) → acceptable at that size (one row per company, ints only); `insights_role_stats` sorts similarly. An index on `(open_count DESC)` serves the `open` sort and the `min_open` filter.

## Migration Plan

- New migration `migrations/0030_insights_company_growth.sql` (table + index). Apply by hand to prod before the worker rebuilds it, then the next `cmd/rollup-company` run (or a manual `systemctl start`) fills it. The endpoint returns an empty list until the first fill — acceptable.
- Rollback: drop the table, revert the worker + handler; `insights_company_stats` is untouched.

## Open Questions

- Default `min_open` value (start at 5, tune from prod once the leaderboard is observed) — a constant, not a blocking decision.
