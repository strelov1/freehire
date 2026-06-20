## Why

Job seekers care most about freshness — a posting from three months ago is
usually noise. The `/jobs` search can sort by posting date but cannot *filter*
by it, so a user who only wants vacancies from the last week still has to scroll
past stale results. A simple "posted within last N days" control closes that gap.

## What Changes

- Add a derived, index-only numeric field `posted_ts` (unix seconds of the job's
  effective posting date) to the Meilisearch job document, and declare it a
  filterable attribute. Sorting is unchanged.
- Accept a new `posted_within_days=N` query parameter on
  `GET /api/v1/jobs/search`. When present and a positive integer, the search is
  filtered to jobs whose `posted_ts` is at or after `now - N*86400`.
- Add a "freshness" control to the web filter sidebar: a single discrete-preset
  slider (Today · 3 days · week · 2 weeks · month · 3 months · Any) that
  round-trips through the URL as `posted_within_days`.
- Operational: the index document/settings change requires one `cmd/reindex`
  after deploy so `posted_ts` is populated on existing jobs. No Postgres
  backfill (the field is derived at index time).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `job-search`: the searchable index gains a `posted_ts` filterable attribute,
  and the public search endpoint gains a `posted_within_days` freshness filter.

## Impact

- Backend: `internal/search/document.go` (new derived field),
  `internal/search/client.go` (filterable attributes), `internal/search/query_filter.go`
  (parse `posted_within_days` → `Gte("posted_ts", cutoff)`). A small shared
  `effectivePostedAt`-to-epoch helper so the index field and `jobview.PostedAt`
  stay one source of truth.
- Frontend: `web/src/lib/filters.svelte.ts` (`postedWithinDays` state + URL
  round-trip + store method), `web/src/lib/facets.ts` (preset list),
  `web/src/lib/components/FiltersPanel.svelte` (the slider).
- Operations: a `cmd/reindex` run after deploy. No DB migration, no Postgres
  backfill, no sort change.
