## Context

The ingest fleet records per-board outcomes in `board_health` (migration
`0006`): one row per `(provider, board)` holding the *latest* result —
`consecutive_failures`, `cooldown_until`, `last_error`, `last_error_at`,
`last_success_at`, `last_ingested_count`, `last_run_at`. It is a runtime-state
sidecar (drives cooldown), not an audit log, so there is no run-by-run history —
only the current snapshot. Today the data escapes only as a per-run summary log
(`ListUnhealthyBoards`); no HTTP surface exposes it.

We want a public `/status` page. Constraints: it is public, so raw error text and
board identifiers must not leak; provider keys are already public (the `sources/`
board files are in git). The frontend is SvelteKit SSR; the backend is Fiber with
a public `/health` endpoint the new route sits beside.

## Goals / Non-Goals

**Goals:**
- A public per-provider health rollup, derived entirely from the existing
  `board_health` snapshot — no new tables, no writes.
- A clear operational/degraded/down signal per provider plus an overall banner,
  robust to noise on large fleets (thousands of boards).
- Sanitized output: no `last_error`, no board identifiers.

**Non-Goals:**
- Run-by-run history / time-series (that would need a new `ingest_run` table and
  a write on every crawl — deferred; noted as a future seam).
- Per-board drill-down in the public page.
- Any change to ingest, cooldown, or recording behavior.

## Decisions

**Rollup in SQL, status policy in Go.** A new sqlc query `ProviderHealthRollup`
does the `GROUP BY provider` aggregation (counts via `FILTER`, `max` timestamps,
`sum` ingested). The operational/degraded/down classification lives in a pure Go
function, matching the codebase convention that policy (like backoff) lives in Go
while SQL stays declarative. This keeps the thresholds unit-testable in isolation.

**Thresholds: 90% / 10% healthy fraction + 48h freshness.** Healthy fraction
`healthy_boards / total_boards`: `operational` ≥ 0.9 (and fresh), `down` ≤ 0.1 or
stale (no success in 48h), `degraded` otherwise. A fraction-based rule keeps a
single failing board out of "degraded" on a 6000-board provider (Workday), which
a strict any-failure rule could not. Constants so they are easy to tune.
*Alternative considered:* freshness-only (simplest) — rejected because it hides
partial breakage; strict any-failure — rejected as perpetually yellow on big
fleets.

**Overall = worst provider status.** Standard status-page semantics; empty fleet
→ `operational` (nothing is broken).

**Public unauthenticated endpoint, sanitized DTO.** `GET /api/v1/status` sits
beside `/health` (no auth middleware). The handler projects the rollup into a DTO
that simply omits `last_error` and board identifiers — sanitization by
construction, not by filtering, so a leak can't happen by forgetting a field.

**Provider display name = title-cased key.** No separate label registry (YAGNI);
`greenhouse` → `Greenhouse` at render time. A registry can come later if labels
need to diverge from keys.

## Risks / Trade-offs

- **Boards never crawled don't appear** → the page reflects "what we're actively
  tracking", not the full YAML catalog. Acceptable for a health view; documented.
- **Snapshot, not history** → a provider that flapped and recovered shows green;
  the page answers "healthy now?", not "was it flaky?". Accepted per Non-Goals;
  the future `ingest_run` table is the seam if history is wanted.
- **Fixed 48h freshness vs varied cadences** → some crons run every 6h, some
  daily; 48h is a lenient common denominator that won't false-alarm. If a
  provider legitimately runs less often than 48h, it would read `down` — none
  currently do; revisit per-provider cadence only if that changes.
- **Public counts reveal fleet size** → provider board counts become public.
  Low sensitivity (sources are open); accepted.

## Migration Plan

No DB migration (table exists). Ship backend endpoint + query, then the web page.
Rollback is removing the route and page — no data or schema to revert.
