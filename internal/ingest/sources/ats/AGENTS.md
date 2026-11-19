# ATS adapters (`sources/ats`)

## Scope

The platform-keyed ATS family: one board is one employer's tenant on a hosted ATS, addressed
by the tenant's slug, read through the platform's public JSON board API. Today: `greenhouse`,
`lever`, `ashby` (plus `ashbygraphql`, its GraphQL fallback for boards the REST API refuses)
and `recruitee`. The rest of the family still lives in `sources` and moves here one PR at a
time (#4586).

## Always true

- **Import `sources/kit` (and `applyform`, for `Job.ApplyForm`) and nothing else of ingest.**
  `sources` imports this package for its registry, so importing `sources` back is a cycle;
  pipeline, boardcatalog and the other ingest packages are callers of adapters, never their
  dependencies (`.golangci.yml`, rule `sources-families`).
- **Registering is still one line in `sources.All`** (`registry.go`). An adapter here is
  exported (`NewGreenhouse`), not registered by itself.
- **The board-API adapters here are `FullBoardListing` and `EmptyBoardListing`**
  (`ashbygraphql`, a per-posting fallback, carries neither): each listing returns the board's
  whole posting array, and an answer without that array fails `Fetch` (`kit.ErrListingAbsent`).
  A new member earns both on the same evidence or says in a comment which it lacks and why.
  `TestMovedATSAdaptersKeepTheirMarkers` and `empty_board_listing_test.go` in `sources` hold
  the registry to it.
- **Capability markers are exported methods** (`func (greenhouse) FullBoardListing() {}`). The
  marker interfaces are in `sources/markers.go`; read their doc comments before granting one.
- Provider notes: [greenhouse](../providers/greenhouse.md), [recruitee](../providers/recruitee.md).
  The rules every adapter meets are in [the sources reference](../AGENT-REFERENCE.md).
