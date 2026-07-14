## Why

freehire positions itself as an open, free "search engine for jobs", but that claim
is asserted, not shown. An "open startup" transparency page — live catalogue and
project metrics on one public URL, in the spirit of cal.com/open — turns the
positioning into proof, doubles as launch content (Product Hunt / HN), and, because
every figure links to the public API endpoint that produced it, becomes a live
demonstration of the API-first / agent-friendly story.

## What Changes

- Add a public **`/open`** page (SSR, unauthenticated) rendering live freehire
  metrics in five sections:
  - **A. Catalogue scale** — open jobs, companies, ATS platforms, Telegram channels.
  - **B. Catalogue movement** — added vs removed over time, reusing the existing
    `/trends` jobs-activity bar chart.
  - **C. What's inside** — facet distributions (top countries, top skills, remote
    share, seniority split) from the existing `/jobs/facets` backend.
  - **D. Open source** — GitHub stars/forks/contributors + MIT badge + a
    "add a source = one PR" call to action, fetched from the GitHub API.
  - **F. Member growth** — cumulative registrations over time, a `/trends`-style
    chart backed by a new public endpoint.
- Add a new public endpoint **`GET /api/v1/stats/user-growth`** — a per-day
  cumulative count of user registrations, computed on the fly from `users.created_at`
  (no rollup table, no worker).
- Each headline figure links to the public API endpoint that produced it.
- Add an **`/open`** link in the site footer/nav.

Non-goals (deliberately deferred): traffic/analytics (GA4), revenue/burn, and any
third-party metric verification. Data is served live from our own API and the
GitHub API; the page degrades per-section so a failing upstream never breaks it.

## Capabilities

### New Capabilities
- `open-transparency-page`: the public `/open` page that aggregates live catalogue
  scale, catalogue movement, facet distributions, open-source stats, and member
  growth into one SSR transparency dashboard, with each figure linking to its API
  source and best-effort per-section degradation.
- `user-growth-stats`: a public `GET /api/v1/stats/user-growth` endpoint returning
  the cumulative count of registered members over time (per-day series), computed
  directly from `users.created_at`.

### Modified Capabilities
<!-- None: the page consumes job-activity-stats, companies, and deterministic-facets
     read-only without changing their requirements. -->

## Impact

- **Frontend (`web/`)**: new `src/routes/open/+page.svelte` + `+page.server.ts`
  (SSR load fanning out to the totals, jobs-activity, facets, user-growth, and
  GitHub API calls, best-effort); reuse of the HomeView stat-strip and the
  `/trends` chart component; a new footer/nav link; `src/lib/api.ts` gains a
  `userGrowth` method.
- **Backend (`internal/`)**: new `JobsActivity`-sibling handler for
  `/stats/user-growth`; a new hand-written query in
  `internal/db/queries/stats.sql` (→ `make sqlc`); route wired in
  `handler.Register` as an unauthenticated public read.
- **Constants**: ATS platform count (98) and Telegram channel count (87) are repo
  constants surfaced on the page, mirroring the homepage stat-strip.
- **No migrations, no new env, no new worker.**
