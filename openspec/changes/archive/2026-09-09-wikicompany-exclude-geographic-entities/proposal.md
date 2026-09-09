## Why

`company-info-wikipedia-backfill`'s production dry-run (task 4.2) caught a false
positive the research spike never surfaced: `Lookup("Nissan")` resolved to
Q270195, a commune in Hérault, France — not the automobile manufacturer.
Wikidata's `P279*` subclass hierarchy is multi-parent, and a commune's class
apparently also descends from something under `organization` (Q43229), so the
existing positive-only walk (`wdt:P31/wdt:P279*` reaching an organization anchor)
accepted it. Confirmed live against `query.wikidata.org`: Q270195 reaches both
`Q2221906` (geographic location) and `Q56061` (administrative territorial
entity), while none of the spike's confirmed-good matches (Paladin Energy,
Hitachi Energy, Royal Bank of Canada, CACI) reach either. This must be fixed
before `--apply` runs at any scale, since a same-named place would otherwise
silently corrupt a real company's `tagline`.

## What Changes

- `buildOrganizationCheckQuery` now also asserts, in the same `ASK` query via
  `FILTER NOT EXISTS`, that the candidate is NOT transitively an instance of a
  geographic or administrative-territorial class — a match must pass the
  positive organization walk AND fail this negative one.
- No change to the worker's behavior contract: a rejected match is still
  handled exactly as any other rejection (checkpoint-only write, no error).

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `company-info`: tightens the existing "confidently typed as company or
  organization" requirement to also exclude geographic/administrative entities
  reachable through the same multi-parent hierarchy.

## Impact

- `internal/job/wikicompany/query.go` only. No schema, API, or worker-contract
  change — `cmd/backfill-company-info-wikipedia` calls the same `Lookup`
  method and handles `nil` (no match) exactly as before.
