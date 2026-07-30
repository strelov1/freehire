## 1. Vocabulary

- [x] 1.1 RED: in `internal/vocab/vocab_test.go`, assert `engineering_design` is a member of `CategoryValues` and of `NonTechCategories`; run `go test ./internal/vocab/...` and watch it fail (the partition test also fails until membership is assigned).
- [x] 1.2 GREEN: add `engineering_design` to `vocab.CategoryValues` (next to `design`) and to `vocab.NonTechCategories`, documenting in the comment why engineering draughting is surfaced but kept off the LLM/embedding budget. `go test ./internal/vocab/...` green, including the partition assertion.

## 2. Title classification (TDD)

- [x] 2.1 RED: in `internal/classify/classify_test.go`, add cases — `Mechanical Design Engineer`, `Electrical Design Engineer`, `Civil Design Engineer`, `Piping Designer`, `Design Engineer` → `engineering_design`; the silicon family (`PCB`/`Physical`/`VLSI`/`ASIC Design Engineer`) → `hardware`; the software-anchored forms (`Software Design Engineer`) → no category; `Product Design Engineer`, `Design Systems Engineer`, `UI Engineer` → `design`; and regression guards that the neighbours are untouched — `Hardware Design Engineer` → `hardware`, `Content Designer` → `technical_writing`, `UX Writer` → `technical_writing`, `Senior Product Designer` / `UX Designer` → `design`. Run and watch the new cases fail.
- [x] 2.2 GREEN: in `internal/classify/dictionaries.go`, insert the product-marker block then the `engineering_design` alias block immediately before the existing `designer`/`design` entries, per design.md §2. `go test ./internal/classify/...` green.
- [x] 2.3 RED→GREEN: in `internal/jobderive` tests, assert an `engineering_design` job derives `is_tech=false` while `Senior Mechanical Engineer` (no category) stays unknown; the derivation already reads `vocab.NonTechCategories`, so this should pass once 1.2 lands — if it does not, fix the derivation.

## 3. Roles (TDD)

- [x] 3.1 RED: in `internal/roletag/roletag_test.go`, assert `categoryNoun` covers `engineering_design` (bare role + `senior_engineering_design` composite), that `Senior Mechanical Design Engineer` yields `mechanical_designer` and **not** `design_engineer` (longest-alias-first), that `Senior Visual Designer` yields `visual_designer` + `senior_visual_designer`, and that `Creative Director` yields no graded composite (nonGradeable).
- [x] 3.2 GREEN: add `categoryNoun["engineering_design"] = "Engineering Designer"`, the product roles (`visual_designer`, `brand_designer`, `motion_designer`, `web_designer`, `industrial_designer`, `ux_researcher`, `art_director`, `creative_director`, `design_ops`, `design_engineer`) and the engineering ones (`mechanical_designer`, `electrical_designer`, `civil_designer`, `drafter`, `bim_specialist`, and — inside `hardware` — `pcb_designer`, `chip_designer`) to `namedRoleTable`, with `art_director`/`creative_director`/`design_ops` in `nonGradeable`. `go test ./internal/roletag/...` green, including the catalog-completeness assertions.

## 4. Skills (TDD)

- [x] 4.1 RED: in `internal/skilltag/skilltag_test.go`, assert a design description ("Figma and Adobe Illustrator, building prototypes, design systems, user research") yields `figma, illustrator, prototyping, user-research`; a CAD description yields `solidworks, creo, sketchup`; and the homonym guards — "sketch out ideas" / "the guiding principle" / "eagle-eyed" in otherwise non-technical prose yield nothing, while "Figma and Sketch" does yield `sketch` (corroborated).
- [x] 4.2 GREEN: add the single-token aliases to `wordAliases`, the multi-word ones to `engineeringPhraseAliases`, and put the eleven homonym tokens in `ambiguousWords`, per design.md §4. `go test ./internal/skilltag/...` green.

## 5. Contracts and web

- [x] 5.1 Regenerate the web contracts via `cmd/gen-contracts` so `CATEGORY_VALUES` and `ROLE_LABELS` pick up the new category and roles (never hand-edit the generated file).
- [x] 5.2 Add `engineering_design: 'Engineering Design'` to the full `CATEGORY_LABELS` map in `web/src/lib/insights.ts`. `web/src/lib/labels.ts` needs no entry — that map holds only the values `humanize()` gets wrong, and `humanize('engineering_design')` already yields "Engineering Design".
- [x] 5.3 Verify the web build per the project's web verification (lint + build; no test runner).

## 6. Verify and record operations

- [x] 6.1 `go build ./... && go vet ./... && gofmt -l .` clean, `go test ./...` green.
- [x] 6.2 Sanity-check the new dictionary against live prod titles: pull a sample through the public search API and confirm the engineering titles now resolve to `engineering_design` and the design titles keep `design` (dictionary run locally over fetched titles — no prod write).
- [x] 6.3 Record the post-deploy sequence in the change: `cmd/backfill-derive`, then a full `make reindex` (not incremental — `is_tech` is outside `content_hash`), never stacked with `reindex-companies`.

## 7. Review rounds (three passes, each found real defects)

- [x] 7.1 Restore the deletion veto the category move removed: `ConfirmedNonTech` spares a resolved `engineering_design` (ingest filter, liveness refresh, prune title rule), and `cmd/prune`'s `isBusinessCategory` spares it in the business rule. Tests name the failure each prevents.
- [x] 7.2 Map the value in `CATEGORY_GROUP` (`web/src/lib/filterSections.ts`) — exhaustively type-checked, so the omission was a `svelte-check` error only, with the facet unselectable while lint and build passed. Verify with `svelte-check`, not just lint+build.
- [x] 7.3 Route the whole silicon family to `hardware` (including the hyphenated `mixed-signal`, `rf`, `rfic`, `analogue`, `silicon`, `memory`, `standard cell` spellings) in both the category table and the `chip_designer` role.
- [x] 7.4 Add the `categoryNone` blind entry for `software design engineer` — as a TABLE ENTRY, not a pre-match mask: masking exposed the business aliases further down the table (`Software Design Engineer - Sales Tools` → `sales`) and blanked the placement that vetoes deletion for `HVAC Systems Design Engineer`. Guard every escape route (`Parse`, `Categories`, `CategoryAliases`) with a test.
- [x] 7.5 Skill precision, second-order: drop `design system(s)`, `visual design`, `solid edge` and bare `after effects` (ordinary prose, and a phrase always matches STRONG, so it lifts the gate off weak words beside it); move `typography`, `illustrator`, `canva`, `lottie`, `wireframes`, `invision`, `prototyping` behind the gate. Each has a negative test in its real false context.
- [x] 7.6 Add the `drafter` and `bim_specialist` roles, cover every previously unwitnessed alias with a test, and sync the OpenSpec artifacts with what the code does.
