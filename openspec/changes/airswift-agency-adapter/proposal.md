## Why

Second of the four agency adapters from
`docs/superpowers/specs/2026-10-08-oilgas-agency-adapters-design.md`. Airswift
(airswift.com) is an international oil & gas / engineering recruitment agency with an
open sitemap and `JobPosting` ld+json on every live posting, no bot defense.

## What Changes

- Add a new `airswift` ingest source adapter (`internal/ingest/sources/airswift.go`),
  the `dataart.go` shape: `sitemap.xml` (flat urlset, ~1841 `/jobs/<slug>-<id>` entries)
  to enumerate, per-posting `GetHTML` + `JobPosting` ld+json to map.
- Boardless, single-company: `Company` is the constant `"Airswift"` — confirmed live
  that `hiringOrganization.name` is the literal string `"Airswift"` on every posting
  (standard recruitment-agency anonymization, same as the whole batch).
- **Expired postings are dropped, not marked unreadable.** ~55-60% of sitemap entries
  are closed roles that still return `HTTP 200` with no `JobPosting` block, but the page
  itself states it: a `c-jobs-article-expired` element containing "no longer accepting
  applicants" (confirmed live, exact class name). The adapter detects that marker and
  drops the posting outright — the same category as a confirmed-gone 404 in `bayt.go`,
  not an ambiguous fetch failure.
- Register `airswift` in `internal/ingest/sources/registry.go`'s `All()`.
- After review and merge, add the live board via `cmd/add-board --provider=airswift
  --company='Airswift' --apply` (boardless) on the prod host.

## Capabilities

### New Capabilities

- `airswift-source`: enumerates Airswift's sitemap and maps each live (non-expired)
  posting's `JobPosting` ld+json into the catalogue as a boardless, single-company
  source.

### Modified Capabilities

(none)

## Impact

- New file `internal/ingest/sources/airswift.go` (+ `airswift_test.go`).
- One new line in `registry.go`'s `All()`.
- One new `boards` row post-merge (data, not code).
- No change to any existing adapter or shared helper.
