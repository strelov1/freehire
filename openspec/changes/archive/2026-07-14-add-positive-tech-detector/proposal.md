## Why

The `is_tech` signal is asymmetric: `false` is set by two mechanisms (the non-tech category blacklist **and** the `classify.IsNonTech` title dictionary), but `true` is set by only one — a recognized technical **category** from the title. Generic tech titles that resolve no sub-category ("Software Engineer II", "Web3 Developer", "Salesforce Developer", "IT Administrator") therefore fall into `unknown`, not `tech`. On prod this leaves the `is_tech=tech` filter a severe undercount (294K) while ~59.5% of the catalogue is `unknown` — a bucket a title-regex shows is ≥9.2% (~170K) obviously-tech. Users filtering "Tech" miss most tech jobs.

## What Changes

- Add a positive **tech title detector** `classify.IsTech(title)` — a curated whole-word dictionary of confident software/IT role terms (software/web/backend/frontend/fullstack/mobile developer, programmer, devops, sre, data scientist, ml engineer, system administrator, cloud/security/qa engineer, `<language>` developer, …), same "never guess" doctrine as `IsNonTech`. It deliberately excludes **bare "engineer"/"analyst"**, which the prod sample shows are dominated by non-software roles (Mechanical/Manufacturing/Drainage/Project Engineer, Geologist).
- Make `jobderive.deriveIsTech` symmetric: `true` when the derived category is technical **OR** `IsTech(title)` fires; else `false` for a non-tech category / `IsNonTech`; else `nil`. Technical evidence still wins over non-technical.
- Re-derive existing jobs (`cmd/backfill-derive`) + reindex so the reclaimed `unknown→tech` reaches the live facet.

## Capabilities

### Modified Capabilities

- `tech-classification`: add the positive tech-title detector and fold it into the `is_tech` derivation (an additional `true` source), tightening the definition of the tri-state.

## Impact

- **Code:** `internal/classify` (new `IsTech` + tech dictionary), `internal/jobderive` (symmetric `deriveIsTech`). No schema change, no wire/contract change — `is_tech` column, jobview field, facet, and filter are unchanged; only more jobs resolve to `tech`.
- **Ops:** after deploy, `cmd/backfill-derive` + reindex (same as the original is_tech ship). Measure the new `true/false/null` split.
