## Context

`user_jobs` already records one row per (user, job) with `viewed_at` (NOT NULL,
defaults to now on RecordView), `applied_at` (nullable), and `saved_at` (nullable).
The `/open` page and its SSR fan-out (`Promise.allSettled`) plus the `/stats/*`
handler family are already in place from the transparency-page change, so this is a
near-mechanical sibling of `/stats/user-growth`.

## Goals / Non-Goals

**Goals:** one cheap public endpoint for aggregate engagement counts; one additive
`/open` section; consistent with the existing stats endpoints and best-effort page.

**Non-Goals:** per-day time series (totals are enough for v1); search-query counting
(needs write-path instrumentation, deferred); any per-user breakdown.

## Decisions

**1. Single `:one` aggregate query with FILTER.** `GetEngagementStats` returns one
row of three ints via `count(*) FILTER (WHERE … IS NOT NULL)` over `user_jobs`.
Cheap, no rollup, mirrors the on-the-fly member-growth decision. Note `viewed_at`
is NOT NULL, so "viewed" equals the total interaction rows — an accurate view count
given RecordView is the row's entry point.

**2. Totals, not a time series.** A stat-strip (saved · applied · viewed) is the
right density for v1 and keeps the query trivial. A time series can be added later
the same way `user-growth` was, if wanted.

**3. Best-effort, like every other `/open` leg.** The engagement fetch joins the
existing `Promise.allSettled` fan-out; a failure drops only its section.

## Risks / Trade-offs

- **Absolute numbers are small pre-launch** → same honest-transparency trade-off as
  member growth; no mitigation needed.
- **"Viewed" counts only signed-in views** (anonymous views aren't in `user_jobs`)
  → acceptable and clearly an engagement (not traffic) metric; the section is
  labelled accordingly.

## Migration Plan

No migration, env, or worker. Ship the endpoint (safe ahead of the page), then the
page section. Rollback = revert the PR.
