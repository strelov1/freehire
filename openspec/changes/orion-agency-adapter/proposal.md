## Why

Part of the oil & gas recruitment-agency batch
(`docs/superpowers/specs/2026-10-08-oilgas-agency-adapters-design.md`). Orion Group
(orionjobs.com) is the simplest of the four: an open, unauthenticated JSON API with no bot
defense, ~220 current oil & gas / engineering vacancies.

## What Changes

- Add a new `orion` ingest source adapter (`internal/ingest/sources/orion.go`) that pages
  `GET https://www.orionjobs.com/api/recruitment/job/data/?folder=uk&hasexpired=false&page=N`
  (10 results/page) until a page returns fewer than 10 results, mapping each entry
  directly to a `Job` — the listing payload already carries the full description, so no
  separate detail fetch is needed.
- Boardless, single-company: `Company` is the constant `"Orion Group"` — the API never
  names a real end client (confirmed live), matching the agency-anonymization pattern the
  design doc establishes for this whole batch.
- Register `orion` in `internal/ingest/sources/registry.go`'s `All()`.
- After code review and merge, add the live board via `cmd/add-board --provider=orion
  --company='Orion Group' --apply` (boardless, no `--board` value) on the prod host.

## Capabilities

### New Capabilities

- `orion-source`: pages Orion Group's open recruitment JSON API into the catalogue as a
  boardless, single-company source.

### Modified Capabilities

(none)

## Impact

- New file `internal/ingest/sources/orion.go` (+ `orion_test.go`).
- One new line in `registry.go`'s `All()`.
- One new `boards` row post-merge (data, not code).
- No change to any existing adapter or shared helper.
