## Why

Meta (metacareers.com) is a high-value FANG employer that was previously written off as
unreachable. Live recon corrected that: the block is **not** by IP and **not** a
persisted-query GraphQL wall — it is a **TLS-fingerprint (JA3) block**. A real Chrome on our
prod host loads the pages fine, while Go `net/http`/curl get `HTTP 400` with
`proxy-status: http_request_error` on every path, because their ClientHello is not Chrome's.
Once that one barrier is crossed, Meta serves a clean plain-HTML job sitemap and renders every
job page with a schema.org `JobPosting` in `application/ld+json` — the exact list→detail shape
the existing `successfactors` adapter already handles.

## What Changes

- Add a `meta` source adapter (`internal/sources/metacareers.go`) speaking the existing
  `Source` interface, registered with one `NewMetaCareers(...)` line in `sources.All`. It is a
  **boardless single-company** source (like `google`/`amazon`/`uber`): one `sources/custom.yml`
  entry `company: Meta`, `provider: meta`, no board.
- It follows the established **list → detail** pattern: enumerate jobs from
  `GET https://www.metacareers.com/jobsearch/sitemap.xml` (each `<url>` carries a
  `job_details/<id>` `<loc>` and a `<lastmod>`), then GET each job page and extract the title +
  description from its `application/ld+json` `JobPosting` via the shared `ldJobPosting()` helper,
  fanned out with the shared `fetchDetails` bounded-concurrency helper.
- **Add a Chrome-fingerprint HTTP client (`github.com/refraction-networking/utls`), scoped only
  to the Meta adapter.** `sources.All` builds one uTLS-backed `*Client` and passes it solely to
  `NewMetaCareers`; the other 60+ adapters keep the plain shared client unchanged. The uTLS
  client layers over the existing `safehttp` SSRF-guarded dialer, so the SSRF guard is preserved.
- **Map ld+json fields:** `title`; `description` (sanitized HTML, like the other detail
  adapters); `datePosted` → `posted_at` (more accurate than the sitemap `<lastmod>`, which is a
  fallback); `jobLocation[].name` (first entry) → `location`. **The `jobLocation[].address.*`
  fields are broken in Meta's markup** (they repeat `"Aiken, SC"`/`"USA"` for every location), so
  the adapter reads `jobLocation[].name` only and never the address sub-object.
- ID = the numeric segment of the `/job_details/<id>/` URL (the pipeline namespaces it under
  `source = "meta"`).

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `meta` is a registered, boardless provider — a
  sitemap-enumerated, ld+json-detail adapter served over a Chrome-fingerprint (uTLS) transport
  scoped to this adapter, yielding the normalized job shape with a sanitized-HTML description.

## Impact

- **New code**: `internal/sources/metacareers.go` + `metacareers_test.go`; one registration line
  in `sources.All`; a Chrome-fingerprint client constructor (uTLS transport over the `safehttp`
  guarded dialer).
- **Dependencies**: add `github.com/refraction-networking/utls` (pure Go, no headless browser).
  No new system dependencies.
- **Config**: one new `sources/custom.yml` entry (`company: Meta`, `provider: meta`). No new env
  vars. New adapter ⇒ a full image rebuild + a cron line (not a sources-only rsync).
- **DB**: none — reuses `UpsertJob` (`source = "meta"`, namespaced `external_id`).
- **Out of scope / known seams**:
  - **Sitemap completeness** — `jobsearch/sitemap.xml` showed 603 entries; whether that is the
    full catalogue or a shard/recent-window cap (Meta has far more openings) is verified during
    implementation (check for a sitemap index / shards). Out of scope to enumerate every
    historical posting if Meta itself does not list them.
  - **HTTP/2 fingerprinting** — Meta's edge may fingerprint h2 as well as TLS; the first
    implementation task is a live spike proving a uTLS request returns 200 from our host before
    the adapter is built on top.
  - Structured multi-location handling — a Meta posting can list many cities; the adapter takes
    the first `jobLocation[].name` and lets enrichment refine the rest.
