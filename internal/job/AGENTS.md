# internal/job

The posting itself: its identity and fingerprints, the derived facets, the public wire projection, deduplication, the ghost/reality signals, and the curated collections over it.

**Layer 5 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate` — and itself.

Must NOT import: `application`, `search`, `engage`, `ingest`, `api`.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`applydate` `collections` `ghost` `ghostreport` `job` `jobdedup` `jobderive` `jobfacts` `jobhash` `jobreality` `jobview` `liveness` `outboundurl` `privatejob` `reqextract` `silence` `verdict` `ycdir`
