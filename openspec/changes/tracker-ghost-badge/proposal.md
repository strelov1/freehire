## Why

The tracked-jobs listing (`GET /me/tracking`) never shows the ghost-job badge, even though the same signal is already computed and shown on every other listing surface (the plain `/jobs` listing, the main filtered search). `internal/application/userjob/AGENTS.md` names this explicitly as a gap: the tracking path has never made the attach call. It matters most exactly for a candidate who already applied — they have the most reason to learn a posting they're waiting on has since been flagged.

## What Changes

- `jobview.Card` gains a `Ghost *jobview.Ghost` field (omitted when there is nothing to say), mirroring `jobview.Job`'s own field.
- `ListTrackedJobs` (`internal/api/handler/me_tracking.go`) attaches the ghost signal to each card, the same best-effort way `jobs.go`/`search.go` already do for their own listings — reusing the existing shared `ghostEvidenceFor` helper and `ListJobGhostStamps` query, not duplicating them.
- The signal's `RealityClass` ingredient is read from a targeted Meilisearch lookup (`id IN [...]`, a new `search.In` filter helper mirroring the existing `search.NotIn`) rather than recomputed from the job's description — the tracker's card query deliberately does not read descriptions (a measured, tested optimization; see Impact), and reality classification needs the raw text. This mirrors exactly how the search-backed listing already gets `RealityClass` for free from a Meilisearch hit's embedded `Reality` field instead of recomputing it.
- **Not adding a `Reality` field to `Card`.** Neither existing listing surface (`/jobs`, search) actually serves `Reality` on a card today — both use it only as an internal ingredient of `ClassifyGhost`, never assign it to the response. Only the job detail page shows `Reality`. This change follows that existing convention rather than introducing a new one.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `user-job-tracking`: the "Listing a user's job interactions" requirement's card-carries list gains `ghost`, and the listing gains a scenario for it — without changing the existing "no description" guarantee.

## Impact

- `internal/job/jobview/card.go`: new `Ghost` field.
- `internal/search/search/filter.go`: new `In(attr string, ids []int64) string`, mirroring `NotIn`.
- `internal/api/handler/user_jobs.go`: `trackingHandlers` gains a `queries *db.Queries` field (already threaded through the constructor, just never stored), matching `jobsHandlers`/`searchHandlers`.
- `internal/api/handler/me_tracking.go`: `ListTrackedJobs` gains a ghost-attach pass over the page's cards.
- No migration, no new query beyond what `ghostEvidenceFor`/`ListJobGhostStamps` already run for other surfaces.
- No change to the listing's own SQL — it still reads only the card's columns; `TestMeasureBoardLoad` (`internal/api/handler/me_tracking_load_measure_test.go`) is unaffected, since nothing here adds a description read.
