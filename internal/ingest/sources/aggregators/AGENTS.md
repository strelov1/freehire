# Aggregator adapters (`sources/aggregators`)

## Scope

The multi-employer feeds: job boards and public employment services that list postings from
many companies, so each posting names its own employer. Today: `adzuna` (keyed search API),
`indeed` (a keyword search read through the crawl host's browser), `eures` (the EU network, board = country), `jobnet` (Denmark), `jobtech` (Sweden's JobSearch
stream) and `nofluffjobs` (Poland). The rest of the family (the SEEK network, the Russian, BR
and PL boards) still lives in `sources` and moves here one PR at a time (#4586).

## Always true

- **Every adapter here carries `Aggregator()`**: it keeps the source in the source facet and
  in the cross-source dedup pass's aggregator set. `TestMovedAggregatorsKeepTheirRegistryStanding`
  in `sources` holds the registry to it.
- **How a removal is detected differs per adapter, and is declared by marker, not assumed by
  family**: `jobnet` lists its whole catalogue (`FullCatalog`), `eures` reads each country
  board to its stated total (`FullBoardListing`), `jobtech` closes its own postings from its
  stream (`SelfClosing`), `adzuna` reads only the newest slice and leaves removals to
  `cmd/adzuna-liveness` (`LivenessProbed`, a 14-day `SweepGrace`), `indeed` (no probe can read a
  posting) closes on a 30-day `SweepGrace` and `cmd/liveness`'s age rule, and `nofluffjobs` relies
  on the default sweep. Read the marker's doc comment in `sources/markers.go` before granting
  one to a new member.
- **Import `sources/kit` (and `applyform`) and nothing else of ingest**, the same rule as
  every family (`.golangci.yml`, rule `sources-families`).
- **`adzuna` is credential-gated**: `sources.All` registers it only with
  `ADZUNA_APP_ID`/`ADZUNA_APP_KEY` set (`registry_special.go`); the taxonomy registry always
  carries it.
- Provider notes: [adzuna](../providers/adzuna.md), [indeed](../providers/indeed.md), [jobnet](../providers/jobnet.md).
