## Why

Four ATS adapters read the wrong field for a posting's work mode. Ashby is the damaging
case: its `isRemote` flag means "not strictly onsite" and is set on hybrid postings too,
so every hybrid Ashby job entered the catalogue as `work_mode=remote` — 701 of 1097
postings sampled across seven boards, and `sources/ashby.yml` carries 3808 boards. The
board's own "Location Type" comes from `workplaceType`, which the adapter never decoded.
Recruitee, SmartRecruiters, and BambooHR do not lie but stay silent: each carries a
richer signal (`hybrid` flag, `hybrid` flag, `locationType`) that the adapter ignores, so
hybrid and onsite roles arrive with no work mode at all.

## What Changes

- Ashby maps `workplaceType` (`Remote`/`Hybrid`/`OnSite`) to the structured work mode;
  `isRemote` drops to a fallback for boards that leave `workplaceType` unset. A hybrid
  posting is no longer flagged `Remote`.
- Recruitee and SmartRecruiters read the `hybrid` boolean alongside `remote`, via a shared
  `workModeFromRemoteHybrid` helper. Both flags false stays unknown rather than a guessed
  `onsite`, per the dict-only contract.
- BambooHR reads `locationType` (`0` onsite, `1` remote, `2` hybrid). Its public careers
  list leaves `isRemote` null on every posting, so today the adapter emits no work mode at
  all; `isRemote` remains as a fallback.
- **BREAKING** for stored data, not for the API shape: re-ingesting these providers moves
  hybrid Ashby jobs out of the remote filter. A reindex is required for search to follow.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `source-ingest`: the "Ingest persists job geography and work mode" requirement currently
  treats "an explicit remote flag from the ATS API" as a structured work mode. It is
  narrowed: when an ATS exposes both a work-mode field and a boolean flag, the field wins,
  and a flag that only distinguishes onsite from everything else SHALL NOT be read as
  `remote`.

## Impact

- `internal/sources/ashby.go` — `AshbyPosting.WorkplaceType`, `MapAshbyPosting`; shared
  with `internal/linksource/ashby.go`, so link-resolved postings gain the same fix.
- `internal/sources/recruitee.go`, `internal/sources/smartrecruiters.go`,
  `internal/sources/bamboohr.go`, `internal/sources/helpers.go`.
- No schema, API, or config change. Post-merge operations: re-ingest the four providers
  (`UpsertJob` refreshes `work_mode`), then `make reindex` so Meilisearch stops serving
  the stale values.
