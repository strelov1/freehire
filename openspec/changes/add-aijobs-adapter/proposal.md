## Why

aijobs.net is a large AI/ML job aggregator (~47k listings, updated continuously) not currently crawled by any adapter in `internal/sources`. Adding it brings in postings from companies with no ATS presence we already crawl (custom career pages, non-US employers), the same value other aggregator adapters (`bayt`, `whatjobs`, `arbeitnow`) already provide for their niches.

## What Changes

- New read-only adapter `internal/sources/aijobs.go` for provider `aijobs`, crawling `aijobs.net`'s paginated listing (a Django-CSRF-protected `POST /?page=N` endpoint, not a public JSON API) and fetching each newly-seen job's detail page for company name, description, and skills.
- New board file `sources/aijobs.yml` (its own file, not `sources/custom.yml`) with a single boardless placeholder entry, since the source is a single global feed rather than one board per company, and its first-run backlog (~47k postings) makes it a poor fit for the shared hourly `custom.yml` cron slot.
- New env var `AIJOBS_MAX_NEW_PER_RUN` (default 500) bounding how many previously-unseen job detail pages one run fetches, so the initial backlog crawl (and any future catch-up after an outage) doesn't turn one cron run into a multi-thousand-request marathon.
- New HTTP helper in `internal/sources/http.go`: a form-urlencoded POST-with-headers method (the package currently only has JSON POST helpers), needed for the CSRF-protected listing endpoint.
- Deliberately dropped: the site's own salary figures (labelled "(estimate)", aggregator-computed rather than employer-provided) and the gated PRO-only original company name / employer apply URL — the adapter derives a company display name from the `/company/<slug>-<id>/` URL instead, and links `Job.URL` to aijobs.net's own job page.

## Capabilities

### New Capabilities
- `aijobs-source`: crawls aijobs.net's job listing (CSRF-protected paginated HTML) and per-job detail pages into normalized `Job` records — provider registration, pagination/backlog bound, and field-mapping rules (company-from-slug, description-from-structured-sections, skills, dropped salary, relative-time `PostedAt`).

### Modified Capabilities
(none — no existing capability's requirements change; the new form-POST HTTP helper is an internal implementation detail of `aijobs-source`, not a separate spec-level capability)

## Impact

- **Affected code**: `internal/sources/aijobs.go` (new), `internal/sources/aijobs_test.go` (new), `internal/sources/http.go` (new form-POST helper), `internal/sources/source.go` / registry wiring (`sources.All`), `sources/aijobs.yml` (new board file).
- **Not affected**: no schema/migration changes, no changes to `cmd/ingest` itself (consumes the registry as-is), no changes to dedup logic (`internal/jobdedup`) — this change relies on the existing `role_fingerprint` cross-source clustering as-is; some duplicate rows against directly-crawled ATS sources are an accepted, known trade-off (same one already accepted for `bayt`/`whatjobs`), not something this change attempts to improve.
- **Out of scope**: wiring the actual cron cadence for `sources/aijobs.yml` in the separate ops/deploy repo (follow-up outside this repo), any new company-identity-matching/dedup logic, salary parsing, and fetching the gated PRO apply URL or company name.
