## Why

Crelate is a recruiting/staffing ATS whose candidate portals expose a keyless public API, yet it is not among freehire's source adapters. Adding it opens a family of staffing-agency boards (Crelate powers many recruiting networks) at zero credential cost, extending catalogue coverage of the ever-jobs ATS list.

## What Changes

- Add a new `crelate` source adapter over the keyless Crelate candidate-portal API (`jobs.crelate.com/api/candidateportal/GetAllJobs`), returning all of a portal's published jobs in one call.
- Register `crelate` in `sources.All` and add a `sources/crelate.yml` board file seeded with one live, validated board (Career Tree Network).
- The board is `<portalSlug>:<organizationId>`: the OrganizationId GUID keys the API; the portal slug builds the human job URL.
- Mark the adapter `aggregator()` — a Crelate portal commonly lists many client companies (the per-posting `CompanyName` is the employer), so the adapter takes each job's company from the posting.

## Capabilities

### New Capabilities
- `crelate-source`: crawl a Crelate candidate portal's published jobs through its keyless public API and normalize them into the catalogue, resolving each posting's employer from the feed.

### Modified Capabilities
<!-- None: this adds an isolated adapter behind the existing Source interface; no existing requirement changes. -->

## Impact

- New files: `internal/sources/crelate.go`, `internal/sources/crelate_test.go`, `sources/crelate.yml`.
- One-line registration in `internal/sources/source.go` (`sources.All`).
- No schema, migration, or API changes. A new hourly ingest timer (`freehire-ingest@crelate`) is installed on prod at deploy, per the standard per-board-file ingest convention.
