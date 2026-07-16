## Why

Strider (onstrider.com) is a LATAM-focused talent marketplace placing remote engineers with
US companies. A spike VALIDATED that its careers site is a HubSpot CMS whose sitemap enumerates
every vacancy URL and whose *open* vacancy pages server-render a schema.org `JobPosting`
ld+json block, while *closed* vacancies drop that markup — giving both a full enumeration
source and a free open/closed liveness signal. Adding it widens LATAM remote coverage at the
cost of a single adapter that reuses the existing sitemap + ld+json machinery.

## What Changes

- Add an `onstrider` source adapter that enumerates canonical vacancy URLs
  (`https://www.onstrider.com/jobs/<slug>-<8hex>`) from `https://www.onstrider.com/sitemap.xml`
  and maps each *open* vacancy page's embedded schema.org `JobPosting` ld+json to a normalized
  `Job` (title, HTML description, `identifier.value` UUID as `ExternalID`, employment type,
  `applicantLocationRequirements` countries, `jobLocationType: TELECOMMUTE` → remote, canonical
  detail URL).
- **Open/closed is the presence of the `JobPosting` block.** The sitemap retains closed and
  `/preview-slug-…/` URLs indefinitely (both still return HTTP 200), so the adapter drops any
  URL that is not a canonical `/jobs/` vacancy and any page without a `JobPosting` block. Jobs
  that flip closed simply stop being emitted and are soft-closed by the standard ingest sweep.
- The adapter is **boardless** and **single-company**: `hiringOrganization.name` is always
  `"Strider"` (the real employer is hidden until a candidate is matched), so every posting maps
  to one company, `Strider`. It is the DataArt shape — sitemap enumerate, per-vacancy detail
  fetch — over the shared `fetchDetails` + `ldJobPosting` helpers.
- Enroll `onstrider` in `sources.All` and in `proxiedProviders`: the site sits behind
  Cloudflare and, like djinni/eightfold, the edge may block the prod datacenter IP, so its
  crawl egresses through `SOURCES_PROXY_URL` when set.
- Add `sources/onstrider.yml` with a single entry (`company: Strider`, boardless) crawled by
  its own `cmd/ingest` cron schedule.

## Capabilities

### New Capabilities
- `onstrider-source`: the `onstrider` adapter — its sitemap enumeration, canonical-URL filter,
  per-vacancy `JobPosting` ld+json mapping, open/closed-via-markup drop rule, boardless
  single-company classification, and proxied-egress enrollment.

### Modified Capabilities
<!-- None. Boardless single-company adapter; inherits the standard ingest sweep and
     board-health machinery unchanged. No spec-level behavior of an existing capability changes. -->

## Impact

- **New code:** `internal/sources/onstrider.go` (+ `_test.go`); `sources/onstrider.yml`.
- **Touched code:** one line in `sources.All` (registry) and one entry in `proxiedProviders`.
- **Ops:** a new `cmd/ingest sources/onstrider.yml` cron schedule (deploy-time, in freehire-ops);
  requires `SOURCES_PROXY_URL` set if the prod IP is blocked.
- **No migrations, no API changes, no new dependencies.**
