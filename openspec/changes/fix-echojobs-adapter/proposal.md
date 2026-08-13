## Why

echojobs.io removed the public JSON API (`/api/jobs` list, `/api/jobs/{handle}` detail) that `internal/sources/echojobs.go` depends on — both endpoints now 404, confirmed from prod's own IP as well as externally, starting around 2026-08-13. The adapter's existing failure handling never drops a posting over a failed detail fetch, so every newly-discovered echojobs posting since the outage started ingests with an empty description instead of failing loudly. One reported job (Doowii "Full-Stack Software Engineer") was patched manually; roughly 600 postings/hour were accumulating the same defect before this proposal. `cmd/backfill-echojobs` and `cmd/liveness/echojobs.go` call the same dead endpoint and are equally broken.

## What Changes

- Replace the echojobs list crawl (`GET /api/jobs?page=N`) with paging `https://echojobs.io/sitemap-jobs/N.xml` (confirmed globally sorted newest-first by `<lastmod>`), stopping once a shard's postings age past the existing 14-day freshness window.
- Replace the echojobs detail fetch (`GET /api/jobs/<handle>`, dead) with `GET /job/<slug>` (a plain HTML page) plus parsing the embedded `<script type="application/ld+json">` block whose `"@type":"JobPosting"` — the schema.org structured-data block the site publishes for Google for Jobs, confirmed to carry title, company, full description, remote signal (`jobLocationType`), location, skills, and posting date.
- Only fetch full JSON-LD detail for postings not already in the catalogue. Already-seen postings inside the freshness window get a liveness-only `touch()` by external ID (no detail fetch) — keeps per-run request volume close to the old shape instead of re-fetching the ~50,000+ postings the freshness window covers on every run.
- Update `EchoJobsDescription` (used by `cmd/backfill-echojobs`) to the same `GET /job/<slug>` + JSON-LD parse, so the backfill worker can repair rows still carrying an empty description from the outage.
- Update `cmd/liveness/echojobs.go`'s `checkEchoJobsLiveAt` to use `GET /job/<slug>`'s HTTP status (200 live / 404 gone) instead of the old JSON error-body signal, which no longer exists.
- The identifier scheme is unchanged: the sitemap URL's slug is the same job_handle already stored as `ExternalID`, so no re-ingestion or duplicate rows result from the swap.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none — this is an internal data-source swap behind the existing echojobs adapter contract: same freshness window, same identifier scheme, same identity/deduplication/persisted-catalogue behavior. No spec currently tracks echojobs as its own capability. Two field-level exceptions, both documented in design.md: `SeenRefresh` jobs now carry an empty `Title` — see design.md's Refresh strategy decision — and `Job.URL` now points at the echojobs.io page rather than the employer's own ATS link the old adapter stored — see design.md's Risks section.)

## Impact

- `internal/sources/echojobs.go` — crawl, detail fetch, and `EchoJobsDescription` rewritten; `echojobs_test.go` fixtures replaced (sitemap XML + HTML-with-JSON-LD instead of JSON list/detail responses).
- `cmd/liveness/echojobs.go` — liveness check rewritten to the new URL + status-code signal; its test updated to match.
- `cmd/backfill-echojobs` — no code change, but now functional again once `EchoJobsDescription` is fixed; should be re-run after deploy to repair rows accumulated during the outage.
- No schema, API, or dependency changes. No effect on other adapters.
