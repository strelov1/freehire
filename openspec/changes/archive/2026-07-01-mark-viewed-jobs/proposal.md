## Why

A signed-in user browsing the job list has no way to tell which postings they have already opened. They re-open the same vacancies and waste time re-reading them. The view data already exists (`user_jobs.viewed_at` is recorded on every job-detail open), but the browse list never surfaces it.

## What Changes

- Add a read endpoint that returns the set of `public_slug`s the signed-in user has viewed, so the SPA can cross-reference the browse list client-side.
- The SPA dims already-viewed job cards in the list and search results (whole-card opacity reduction, restored to full on hover), gated to signed-in users.
- The public job list/search endpoints stay unauthenticated — viewed state is fetched separately, not joined into the public read path.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `user-job-tracking`: adds a new requirement — the API exposes the set of viewed job slugs for the authenticated caller.
- `web-frontend`: adds a new requirement — the job browse list and search results visually mark jobs the signed-in user has already viewed.

## Impact

- **Backend**: new `ListViewedJobSlugs` query (`internal/db/queries/user_jobs.sql`), regenerated sqlc, new `ListViewedSlugs` handler + route `GET /api/v1/me/jobs/viewed` (`internal/handler/me_jobs.go`, `handler.go`). No change to the `jobs`/`user_jobs` schema and no change to the `jobview.Job` wire shape (so the generated TS contracts are untouched).
- **Frontend**: new API client method, a small reactive viewed-slugs store, and edits to `JobsView.svelte` (load on mount when authed), `JobRow.svelte` (dim when viewed, opt-out prop for the My Jobs surfaces), and `JobView.svelte` (mark viewed after recording).
- **No breaking changes**; unauthenticated browsing is unaffected.
