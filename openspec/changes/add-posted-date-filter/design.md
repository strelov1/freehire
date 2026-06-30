## Context

`/api/v1/jobs/search` (Meilisearch-backed) already supports a numeric range
filter for salary (`Gte("enrichment.salary_min", n)` / `Lte(...)`) and sorts
browse results by `posted_at` descending. The job document carries `posted_at`
as an **RFC3339 string**, derived from `effectivePostedAt(posted, created)` in
`internal/jobview` — the source `posted_at` when present and not in the future,
else `created_at`. Meilisearch range operators (`>=`, `<=`, `TO`) work only on
**numbers**, so the existing string `posted_at` cannot back a date-range filter.

## Goals / Non-Goals

**Goals:**

- Filter `/jobs` search to "posted within the last N days" via a single relative
  parameter `posted_within_days`.
- Reuse the existing salary-range plumbing (`Gte`, `FilterFromValues`, the
  filter sidebar slider pattern) rather than inventing new machinery.
- Keep the effective-posting-date definition single-sourced with `jobview`.

**Non-Goals:**

- A two-ended date range (from–to). Out of scope.
- A separate filter on raw `created_at`. Out of scope.
- Changing sort behavior. Sorting still uses the string `posted_at`.
- Facet counts/distribution for the date control.

## Decisions

### Relative `posted_within_days`, not an absolute cutoff

The API takes a day count; the backend computes `cutoff = now - N*86400` at
request time. **Alternative considered:** the client computes an absolute
timestamp and sends `posted_after=<unix>`. Rejected: a bookmarked/shared "last 7
days" URL would freeze to a stale absolute window, and the URL would carry an
opaque epoch instead of a clean `posted_within_days=7`. The server already
depends on wall-clock time for `effectivePostedAt`, so computing `now` here is
consistent.

### Derived index-only `posted_ts` (unix seconds)

Add `posted_ts int64` to `JobDocument`, set to the epoch of `effectivePostedAt`,
and declare it filterable (not serialized in the public job shape, not added to
sortable attributes). **Alternative considered:** store a numeric timestamp as a
real Postgres column. Rejected as overengineering — the value is fully derived
from existing columns at index time, so a column + migration + backfill buys
nothing the index field doesn't.

### Single source of truth for the effective date

`effectivePostedAt` is currently private in `jobview` and returns a
`pgtype.Timestamptz`. Extract a small exported helper so both the display
`posted_at` (RFC3339) and the new `posted_ts` (epoch) derive from one function,
rather than duplicating the null/future fallback logic in `search`. The exact
helper shape (e.g. an exported `EffectivePostedAt` returning the resolved
`time.Time`, with `search` calling `.Unix()`) is settled during implementation;
the invariant is "one definition, two encodings."

### `now` is injected, not read from a global

`FilterFromValues` is presently a pure function of `url.Values`. To keep the
date branch unit-testable without monkeypatching `time.Now`, the reference time
is passed in (e.g. an added parameter, or a sibling function that takes `now`).
This mirrors how `effectivePostedAt` is the only place that reaches for the
clock.

### Discrete presets, not a free day count

The slider snaps to `[1, 3, 7, 14, 30, 90, null]` (Today → 3 months → Any).
**Alternative considered:** a continuous 1–365 slider like salary. Rejected:
"47 days" is meaningless to a job seeker and harder to land on a round value;
presets read clearly and keep URLs tidy. The slider is placed at the **top** of
`FiltersPanel` because freshness is a primary filter.

## Risks / Trade-offs

- **Reindex required before the filter works on old jobs** → `posted_ts` only
  appears on documents written after the settings change. Mitigation: ship a
  `cmd/reindex` step in the deploy (tasks call it out explicitly); until then the
  filter simply returns fewer/older results, never wrong ones.
- **Relative window depends on server clock at query time** → a result set is
  not reproducible to the exact second across requests. Acceptable and expected
  for a "freshness" control; the alternative (absolute cutoff) trades this for
  staleness, which is worse here.
- **Day-granular sources** → some sources only provide a date, not a time;
  `effectivePostedAt` already normalizes this and the boundary is "within N
  days," so sub-day precision is irrelevant.

## Migration Plan

1. Deploy backend + frontend (no DB migration).
2. Run `cmd/reindex` so `posted_ts` populates existing documents.
3. Rollback: revert the deploy; the orphan `posted_ts` field on documents is
   harmless and is removed on the next reindex from the reverted code.

## Open Questions

None — design approved in brainstorming.
