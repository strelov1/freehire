## Why

The content-dedup collapse hides a role cluster's reposts behind one canonical card and
surfaces an openings count ("929 open copies"), but there is no way to SEE those copies.
For a mass-posted role (e.g. Lowe's Cashier across 1251 stores, T-Bank Представитель
across cities) a seeker wants to pick their own city — each copy keeps its own location
and apply URL, but they are only reachable by a slug nobody has.

## What Changes

- New `GET /api/v1/jobs/:slug/copies` returns the open postings sharing the job's role
  cluster (`company_slug` + `role_fingerprint`) — each with its location, apply URL, and
  public slug, ordered by location, paginated. Public, like the other job reads; the
  anchor itself is included (it is one of the openings).
- The job detail page renders an "N openings across locations" section (component
  `JobCopies.svelte`), listing the cities with links, shown only for a genuinely
  mass-posted role (more than one copy). Degrades to nothing on failure.

## Capabilities

### New Capabilities
- `job-cluster-copies`: the per-city openings list under a collapsed role — the copies
  endpoint and the detail-page section.

### Modified Capabilities
<!-- none -->

## Impact

- `internal/db/queries/jobs.sql` — `ListRoleClusterCopies`; regenerate sqlc.
- `internal/handler/copies.go` + route in `handler.go`.
- `web/` — `getJobCopies` API client, detail loader fetch, `JobCopies.svelte`.
- No schema change; reuses the existing `role_fingerprint` cluster.
