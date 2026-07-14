## Context

The site already has the raw material for a transparency page but no page that
gathers it: `/trends` renders `GET /api/v1/stats/jobs-activity` via the
`ActivityBars` component (SSR through `serverApi(fetch)`); `/about` SSRs the
catalogue totals; `GET /api/v1/jobs/facets` returns facet distributions; company
and job totals come back in list-endpoint `meta.total`. The one missing datum is a
member-growth series — there is no per-day registration count today. The GitHub
numbers live outside our system (GitHub REST API). This change is additive: a new
route, one new read endpoint, and reuse of existing components.

## Goals / Non-Goals

**Goals:**
- One public `/open` page proving the "open, free search engine" claim with live data.
- Reuse existing building blocks (`ActivityBars`, the HomeView stat-strip idiom,
  `serverApi`, the `/trends` load pattern) — no new chart library, no redesign.
- Best-effort per-section SSR: any single upstream failing degrades only its section.
- Each figure links to the API endpoint that produced it (API-first proof).

**Non-Goals:**
- Traffic/analytics (GA4), revenue/burn, third-party metric verification (jitsu-style).
- A rollup table or worker for member growth — computed on the fly.
- Realtime/streaming updates; SSR-on-request freshness is enough.

## Decisions

**1. Member growth: on-the-fly SQL, no rollup.**
Add `UserGrowth` as a `JobsActivity` sibling in `internal/handler/stats.go`, backed
by a new hand-written query in `internal/db/queries/stats.sql` that groups
`users.created_at` by UTC day and returns a running cumulative total (window
`SUM() OVER (ORDER BY day)`), then `make sqlc`. At ~hundreds of users this is
trivially cheap; a rollup table (like `job_daily_stats`) would be over-engineering.
Wire it in `handler.Register` as an unauthenticated public read next to
`/stats/jobs-activity`. *Alternative rejected:* mirror the `job_daily_stats`
rollup + `cmd/rollup-stats` — unjustified infrastructure for a tiny table.

**2. Reuse `ActivityBars` for both time series.**
Section B feeds it the jobs-activity points as today. Section F feeds it the
cumulative member series (a monotonically rising bar staircase). Reusing one
component keeps the visual language consistent and avoids a new chart dependency.
*Alternative considered:* a dedicated line chart for cumulative growth — deferred;
not worth a new component for v1.

**3. SSR fan-out with per-section isolation.**
`/open/+page.server.ts` `load` calls `serverApi(fetch)` for totals, jobs-activity,
facets, and user-growth, plus a GitHub fetch, each wrapped so a rejection resolves
to `null` (e.g. `Promise.allSettled` or per-call `try/catch`). The page renders
each section from its own (possibly null) slice, showing a fallback when null. This
directly satisfies the "best-effort per-section degradation" requirement.

**4. GitHub data: server-side fetch, cached, best-effort.**
Fetch `https://api.github.com/repos/strelov1/freehire` (stars, forks, license) and
the contributor count during SSR. Unauthenticated GitHub REST is limited to 60
req/hr per IP, so cache the result in a module-level in-memory memo with a TTL
(~1h) in `+page.server.ts` and set a matching `cache-control` via `setHeaders`, so
repeated loads don't exhaust the budget. On failure the section falls back to a
plain GitHub link. *Alternative rejected:* a server-side scheduled fetch/store —
unneeded for one low-traffic page.

**5. Constants for ATS/Telegram counts.**
The ATS-platform count (98) and Telegram-channel count (87) are repo facts, not DB
rows; surface them as constants on the page exactly as the homepage stat-strip
already does. They change only when adapters/channels are added, i.e. on deploy.

**6. Entry point.**
Add an `/open` link in the footer (and/or the existing nav/menu), alongside the
`/about`, `/trends`, `/blog` links.

## Risks / Trade-offs

- **GitHub rate limit (60/hr unauth)** → module-level TTL cache + `cache-control`;
  worst case the section shows its fallback link, page still renders.
- **Low absolute numbers pre-launch (61 stars, 161 members)** → acceptable: the
  transparency story is the point, and the launch bends the curves (before/after
  is itself content). No mitigation needed beyond honest framing.
- **Constant drift (ATS=98 / TG=87 go stale)** → same trade-off the homepage already
  accepts; refreshed on deploy when sources change. Not load-bearing.
- **SSR fan-out latency (5 upstream calls)** → run them concurrently
  (`Promise.allSettled`); the GitHub call is cached so steady-state is 4 internal
  calls, all already used elsewhere.

## Migration Plan

No DB migration, no new env, no worker. Ship order: (1) backend endpoint +
`make sqlc` + tests, deploy; (2) frontend page consuming it. Because the page is
additive and unauthenticated, rollback is simply reverting the PR. The
`/stats/user-growth` endpoint is safe to deploy ahead of the page.

## Open Questions

- Footer vs nav vs both for the `/open` link — decide during implementation to
  match the existing chrome; not blocking.
