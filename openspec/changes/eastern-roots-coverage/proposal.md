# Eastern-roots coverage

## Why

idagent.pro/companies curates 82 international tech companies with Eastern
European / Russian-speaking roots. An audit of that list against our catalogue
found two gaps:

- Several companies are ingestable via an ATS we already support but were not
  yet in a board file.
- Several ingested (or newly-addable) companies were missing from the
  `eastern-roots` collection membership, so they never receive the tag.

It also surfaced a latent data bug: three eastern-roots companies were merging
into a shared catalogue company row under a generic slug (`ajax`, `vivid`,
`genesis`) alongside unrelated companies, because `company` was a bare generic
name.

## What Changes

- **Add 6 live-validated ATS boards** to existing board files: Chatfuel
  (breezy), Kommo (workable), Jooble (teamtailor), Emerging Travel Group
  (workable), Let's Enhance (teamtailor), Headway Inc (teamtailor).
- **Disambiguate 3 generic-slug merges** by renaming the eastern-roots company
  to a unique name (board unchanged): `lever` `ajax` → **Ajax Systems**,
  `personio` `vivid` → **Vivid Money**, `breezy` `gen-tech` → **Genesis Tech**.
  This both fixes the merge and yields a slug that matches the membership list.
- **Add 10 membership slugs** to `internal/collections/eastern_roots.txt`:
  `1inch`, `chatfuel`, `genesis-tech`, `headway-inc`, `itechart`, `jooble`,
  `kommo`, `let-s-enhance`, `mercuryo`, `reface`.

## Non-goals

- Building new ATS adapters for companies that only expose custom or
  LinkedIn-only careers sites (Kaspersky, Revolut, Grammarly, DataArt,
  SoftServe, Nexters, G5, Fibery, YouHodler, Bitrix24, StarWind, Bitfury,
  Grid Dynamics, Plesk/UKG, Zerion, Ecwid). Each is a separate change.
- No behavior/requirement change to the `source-ingest` or `job-collections`
  capabilities — this is data/content only, so the change carries **no spec
  delta** (archive with `--skip-specs`).

## Impact

- Data files only: `sources/{breezy,lever,personio,teamtailor,workable}.yml`,
  `internal/collections/eastern_roots.txt`.
- Ops to land after merge: ingest the changed board files, run `cmd/reslug`
  (for the 3 renames), run `cmd/import-collections`, then `make reindex`.
