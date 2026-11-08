## Why

Join.com (join.com) powers the career pages of a large segment of European (DACH/SMB)
employers and is one of the four OpenJobs-covered ATS platforms we do not yet ingest. Unlike
OpenJobs' HTML-scrape adapter, Join.com exposes a **public JSON API** (`/api/public/...`) with
a clean list endpoint (authoritative pagination) and a per-job endpoint, so the postings are
reachable as structured JSON — no HTML scraping, no bot-wall.

## What Changes

- Add a `join` source adapter (`internal/sources/join.go`) speaking the existing `Source`
  interface, registered with one `NewJoin(c)` line in `sources.All`.
- It follows the established **list → detail** JSON pattern (like SmartRecruiters/Gem): the
  list endpoint carries no description, so each posting's body comes from its own detail
  request, fanned out with the shared `fetchDetails` bounded-concurrency helper.
  - **List**: `GET https://join.com/api/public/companies/<board>/jobs?page=N&pageSize=100`,
    looping pages `1..pagination.pageCount`. Each item carries `id`, `idParam`, `title`,
    `createdAt`, `city {cityName, countryName}`, and `workplaceType`.
  - **Detail**: `GET https://join.com/api/public/jobs/<id>` for the job body (`description`)
    and the company slug (`company.domain`, used to build the public URL).
- The `sources.yml` `board` value is the **numeric Join company id** (e.g. `167291`), since
  the API keys jobs by company id, not slug (the slug path returns empty). Each entry's config
  comment carries the human-readable slug/name; the company id is read once from the company
  page when adding a board.
- **Join descriptions are Markdown, not HTML.** To honor the "adapter descriptions are
  sanitized HTML" convention, the adapter renders the Markdown `description` to HTML with
  **goldmark** (a standard, dependency-free Go Markdown library) and then `sanitizeHTML`s it —
  the idiomatic library, not a newline/regex shim.
- `external_id` is the API's canonical numeric job `id`; the public `URL` is
  `https://join.com/companies/<company.domain>/<idParam>`. `remote` is `workplaceType ==
  "REMOTE"` (falling back to the shared `isRemote` heuristic); `location` is the list item's
  `city` (locality + country) and may be empty; `posted_at` is `createdAt` (RFC3339).

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities

- `source-ingest`: add a requirement that `join` is a registered provider — a JSON
  list-API-enumerated, JSON-detail adapter yielding the normalized job shape with a
  Markdown-rendered, sanitized-HTML description, consistent with the existing adapters.

## Impact

- **New code**: `internal/sources/join.go` + `join_test.go`; one registration line in
  `sources.All`.
- **Dependencies**: one new direct dependency, `github.com/yuin/goldmark` (Markdown→HTML).
  No transitive third-party additions (goldmark is dependency-free).
- **Config**: one new `sources.yml` file (`sources/join.yml`). No new env vars.
- **DB**: none — reuses `UpsertJob` (`source = "join"`, namespaced `external_id`).
- **Out of scope (known seams)**: structured department/category mapping (left to enrichment);
  the `unifiedDescription` HTML field (a bool / not yet populated on observed jobs — Markdown
  `description` is the reliable body); the remaining two OpenJobs platforms (Breezy, Jobvite),
  added as their own changes one at a time.
