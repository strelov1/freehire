## 1. Vocabulary and the prune protection

- [x] 1.1 Add `industrial_engineering` to `vocab.CategoryValues` and `vocab.NonTechCategories`, with a comment saying what it covers and why it is non-technical.
- [x] 1.2 Add a named set of the non-technical CRAFT categories (`engineering_design`, `industrial_engineering`) to `vocab`, with a test asserting it is a subset of `NonTechCategories`.
- [x] 1.3 Rewrite `cmd/prune/rule.go`'s `isBusinessCategory` to subtract that set instead of the inline `engineering_design` string, with tests that both craft categories are spared and the back-office categories are still deletable.

## 2. Title classification — the English seats

- [x] 2.1 Add the industrial engineering seats → `industrial_engineering`: project, quality, process, manufacturing, production, maintenance, controls, automation, reliability, commissioning, validation, industrial, plant, facilities, site, field, service, supplier quality, safety, environmental, geotechnical engineer. With tests.
- [x] 2.2 Declare the IT lookalikes ABOVE the bare alias: `it engineer` → `software_engineering`, `database engineer` → `devops`, `business intelligence engineer` → `data_analytics`, `electronics engineer` → `hardware`; and `field application engineer` → `solutions_engineering`. With tests.
- [x] 2.3 Bare `engineer`: TRIED and REJECTED. The existing suite pins "Product Engineer", "Growth Engineer", "Staff Engineer" and "Developer Onboarding Engineer" to no category on purpose, and `Categories()` returns every match, so a bare alias would append this category to every engineering title in the catalogue. Same for `reliability engineer`. Both are recorded as deliberate omissions with tests, plus regression tests that every software/data/security/infrastructure discipline is unchanged.

## 3. Title classification — the Russian family

- [x] 3.1 Declare the Russian titles that name another discipline ABOVE the bare token: `инженер-проектировщик` → `engineering_design`, `инженер по защите информации` → `security`. With tests.
- [x] 3.2 Add bare `инженер` → `industrial_engineering` plus `технолог`, with tests covering the hyphenated and prepositional forms (`Инженер-технолог`, `Инженер ПТО`, `Главный инженер`, `Инженер по подготовке производства`).

## 4. Named roles

- [x] 4.1 Add `industrial_engineering` to `roletag.categoryNoun` — without it the bare and graded roles have no label and the coverage test fails.
- [x] 4.2 Add the craft roles (`project_engineer`, `quality_engineer`, `process_engineer`, `maintenance_engineer`, `controls_engineer`, `automation_engineer`, `field_service_engineer`, `industrial_engineer`), with tests that they do not steal from the design crafts.

## 5. Contracts and the two apps

- [x] 5.1 Regenerate `web/src/lib/generated/contracts.ts` via `cmd/gen-contracts`.
- [x] 5.2 Add the label to `web/src/lib/labels.ts`, the section entry to `web/src/lib/filterSections.ts`, and the label to `extension/lib/labels.ts`; run `svelte-check`.

## 6. Verification

- [x] 6.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, `golangci-lint run`; all clean.
- [x] 6.2 Re-run the mining pass against the prod dump and record the measured gain in the change.
