# Aggregator adapters (`sources/aggregators`)

## Scope

The multi-employer feeds: job boards and public employment services that list postings from
many companies, so each posting names its own employer. Today: `adzuna` and `careerjet` (keyed search APIs),
`eures` (the EU network, board = country), `jobnet` (Denmark), `jobtech` (Sweden's JobSearch
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
  `cmd/adzuna-liveness` (`LivenessProbed`, a 14-day `SweepGrace`), and `nofluffjobs` relies
  on the default sweep. Read the marker's doc comment in `sources/markers.go` before granting
  one to a new member.
- **Import `sources/kit` (and `applyform`) and nothing else of ingest**, the same rule as
  every family (`.golangci.yml`, rule `sources-families`).
- **`adzuna` is credential-gated**: `sources.All` registers it only with
  `ADZUNA_APP_ID`/`ADZUNA_APP_KEY` set (`registry_special.go`); the taxonomy registry always
  carries it.
- **`careerjet` is credential-gated** on `CAREERJET_API_KEY` and `CAREERJET_USER_IP` (the server's
  public egress IP, its required `user_ip`). Its urls are click-tracking links, so like `adzuna` it
  cannot be probed: a 14-day `SweepGrace`, and `cmd/liveness` age-expires it
  (`expireDespiteRegisteredPrefixes`).
- Provider notes: [adzuna](../providers/adzuna.md), [careerjet](../providers/careerjet.md), [jobnet](../providers/jobnet.md).
