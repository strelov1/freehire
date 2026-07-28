## Why

Every one of the ~150 crawled providers is multi-tenant SaaS, so the whole ingest fleet is
blind to self-hosted ATS installs — there is no tenant catalogue to enumerate. Reconnaissance
on 2026-07-28 confirmed the gap is real and fillable: 9 live OpenCATS career portals carrying
**260 open jobs** (including 108 at G4S and 95 at Indovision Global), none of them reachable
today. The marginal cost is one adapter plus one prober, reusing the entire existing pipeline.

## What Changes

- Add an `opencats` source adapter that crawls a self-hosted OpenCATS career portal, registered
  in the source registry under provider key `opencats`.
- Add `sources/opencats.yml`, populated by a live harvest run.
- Add an OpenCATS harvest prober with discovery support, so
  `go run ./cmd/harvest-boards opencats` needs no seed file.
- Extend board harvest so a provider without any vendor API can discover candidates from a
  third-party scan index and validate them against the portal's own HTML.

## Capabilities

### New Capabilities
- `opencats-source`: crawling a self-hosted OpenCATS career portal — board identity as
  host plus optional path prefix, routing-invariant parsing, per-posting failure isolation.

### Modified Capabilities
- `board-harvest`: candidate discovery and live-validation currently require the platform's
  official public API. A self-hosted platform has no such API, so the requirement must admit
  a third-party discovery index and portal-HTML validation, including the exclusion of
  candidates that belong to an already-covered sibling provider.

## Impact

- **New code:** `internal/sources/opencats.go` (+ tests),
  `cmd/harvest-boards/opencats_prober.go` (+ tests), `sources/opencats.yml`.
- **Modified:** one registry line in `sources.All`; the prober registry in
  `cmd/harvest-boards`.
- **Unchanged by design:** no migration, no schema change. Cross-install id collisions are
  already handled — `pipeline.jobIdentity` namespaces every `external_id` by board.
- **New external dependency (host tooling only):** the urlscan.io public search API, used by
  the harvest tool. Not reachable from, and not required by, any production worker.
- **Lifecycle:** provider is not self-closing; withdrawn postings close via the 48-hour
  unseen-job sweep.
- **Out of scope:** FreeATS (zero public installs found), application submission.
