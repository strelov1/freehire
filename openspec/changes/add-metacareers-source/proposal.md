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
- **Add a Chrome-fingerprint HTTP client (`github.com/bogdanfinn/tls-client`, profile
  `Chrome_133`), scoped only to the Meta adapter.** A live spike proved plain JA3 spoofing
  (`refraction-networking/utls`) is insufficient — Meta's edge fingerprints the **HTTP/2 layer**
  too and `400`s Go's h2 framer; `tls-client` (which spoofs both JA3 and the Chrome h2
  fingerprint via its bundled `fhttp` net/http fork) returns `200`. `sources.All` builds one
  `tls-client`-backed client and passes it solely to `NewMetaCareers`; the other 60+ adapters keep
  the plain shared client unchanged. The client dials through the existing `safehttp` SSRF-guarded
  dialer (via `tls_client.WithDialer`), so the SSRF guard is preserved.
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
  sitemap-enumerated, ld+json-detail adapter served over a Chrome-fingerprint (`tls-client`)
  transport scoped to this adapter, yielding the normalized job shape with a sanitized-HTML
  description.

## Impact

- **New code**: `internal/sources/metacareers.go` + `metacareers_test.go`; one registration line
  in `sources.All`; a Chrome-fingerprint transport wrapper (`tls-client` over the `safehttp`
  guarded dialer) implementing the `XMLGetter`/`HTMLGetter` roles; an exported
  `safehttp.GuardedDialer`.
- **Dependencies**: add `github.com/bogdanfinn/tls-client` (pure Go, no headless browser; bundles
  the `bogdanfinn/fhttp` net/http fork and a `utls` fork). Confined to the one Meta transport
  wrapper. No new system dependencies.
- **Config**: one new `sources/custom.yml` entry (`company: Meta`, `provider: meta`). No new env
  vars. New adapter ⇒ a full image rebuild + a cron line (not a sources-only rsync).
- **DB**: none — reuses `UpsertJob` (`source = "meta"`, namespaced `external_id`).
- **Out of scope / known seams**:
  - **Sitemap completeness** — resolved by the spike: `jobsearch/sitemap.xml` is a flat list of
    603 entries with no `<sitemap>` shard index, so that is the catalogue this source exposes. Out
    of scope to enumerate postings Meta itself does not list.
  - Structured multi-location handling — a Meta posting can list many cities; the adapter takes
    the first `jobLocation[].name` and lets enrichment refine the rest.
