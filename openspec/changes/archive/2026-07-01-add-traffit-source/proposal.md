## Why

Traffit is a widely-used Polish/CEE Applicant Tracking System hosting each client's
career page on a `<tenant>.traffit.com` subdomain. Every tenant exposes a keyless,
self-contained public JSON list endpoint (`/public/an/list/`) that returns fully
populated postings — title, HTML description, structured geolocation, and a stable
posting id — in one call. We do not ingest it today, so a large pool of live
Polish/CEE IT vacancies is missing from the catalogue, a region where our coverage
is thin.

## What Changes

- Add a `traffit` source adapter (`internal/sources/traffit.go`) over the keyless
  `https://<board>.traffit.com/public/an/list/` JSON API. The board id is the tenant
  subdomain; the adapter is board-based (not boardless) and stays in the source facet.
- The adapter **paginates** (`?limit=100&offset=N`): the endpoint caps a page at 100
  items and defaults to 10, so a single fetch would silently truncate large boards
  (e.g. `jit` has 167 postings). It loops until it has collected `count` items or a
  page comes back empty.
- Register the adapter in `sources.All` (one line).
- Add a `traffitProber` to `cmd/harvest-boards` that validates a seed list of candidate
  tenant slugs against the list endpoint (a real tenant returns JSON with a job count;
  a non-tenant subdomain returns an HTML placeholder, so `GetJSON` fails and the slug
  is skipped — the same shape as the greenhouse/lever probers). Auto-discovery from
  DNS/certificate-transparency is not keyless-feasible (Traffit serves all tenants
  under a wildcard cert and wildcard DNS), so the seed is supplied, not enumerated.
- Add `sources/traffit.yml` seeded with a validated starter set of live tenants
  (~14 tenants, ~385 open postings) so the adapter delivers volume immediately.

## Capabilities

### New Capabilities
- `traffit-source`: crawling one Traffit tenant's public job list into the catalogue,
  and validating candidate Traffit tenant slugs during harvest.

### Modified Capabilities
<!-- none: no existing spec's requirements change -->

## Impact

- New: `internal/sources/traffit.go` (+ test), `cmd/harvest-boards/traffit.go` (+ test),
  `sources/traffit.yml`.
- Modified: `internal/sources/source.go` (one registry line in `sources.All`),
  `cmd/harvest-boards`'s `probers` map (one line).
- Ops: after committing, a `cmd/ingest sources/traffit.yml` cron entry crawls the file;
  no schema/migration change. Incremental search indexing works unchanged (adapter goes
  through `UpsertJob`).
