## 1. Vocabulary

- [x] 1.1 Add `healthcare`, `skilled_trades`, `retail`, `hospitality` to `vocab.CategoryValues` and `vocab.NonTechCategories`, with a comment on each saying what it covers.
- [x] 1.2 Add a test asserting `NonTechCraftCategories` still contains exactly `engineering_design` and `industrial_engineering` — none of the four is craft-protected, and that is what keeps deletion behaviour unchanged.
- [x] 1.3 Add prune tests: a consumer posting at a company with no technical evidence still matches a removal rule, and the same posting at a technical company is still kept.

## 2. Healthcare

- [x] 2.1 Add the English healthcare vocabulary → `healthcare`: registered nurse, nurse practitioner, LPN, CNA, caregiver, home health aide, medical assistant, dental hygienist, dental assistant, pharmacy technician, medication technician, patient coordinator, phlebotomist, physical/occupational therapist, veterinarian. With tests.
- [x] 2.2 Add the Russian medical vocabulary → `healthcare`: bare `врач`, plus `медсестра`, `медбрат`, `фельдшер`, `санитар`, `ветеринар`. With tests covering the hyphenated and postfixed forms.

## 3. Skilled trades

- [x] 3.1 Add the English trades → `skilled_trades`: service/field service/installation/production/diesel/automotive technician, mechanic, electrician, plumber, welder, HVAC technician, machinist, millwright, carpenter, painter. With tests.
- [x] 3.2 Add the Russian trades → `skilled_trades`: `электрик`, `электромонтёр`, `сварщик`, `слесарь`, `плотник`, `механик`, `наладчик`, `маляр`. With tests.
- [x] 3.3 Declare `medication technician` and the other healthcare-qualified technician spellings ABOVE the trades family, with tests.

## 4. Retail and hospitality

- [x] 4.1 Add the retail vocabulary → `retail`: team member, sales associate, cashier, merchandiser, retail service specialist, stock associate, brand ambassador, product demonstrator, store leader, and the grocery clerk family (deli, grocery, produce, bakery, meat). With tests.
- [x] 4.2 Add the hospitality vocabulary → `hospitality`: server, host/hostess, chef, line cook, prep cook, barista, bartender, dishwasher, busser, banquet server, kitchen assistant. With tests.
- [x] 4.3 Add the collision tests the mining pass surfaced: `Делопроизводитель` must not resolve through `водитель`, `Электромеханик` resolves to trades not through a bare `механик` mis-boundary, `Store Driver` is retail, `Medication Technician` is healthcare.

## 5. Named roles

- [x] 5.1 Add `categoryNoun` entries for all four — without them the bare and graded roles have no label and the coverage test fails.
- [x] 5.2 Add the named roles: `nurse`, `caregiver`, `service_technician`, `automotive_technician`, `electrician`, `welder`, `retail_associate`, `cashier`, `server`, `cook`, `barista`. With tests that they do not steal from the industrial or design crafts.

## 6. Contracts and the two apps

- [x] 6.1 Regenerate `web/src/lib/generated/contracts.ts` via `cmd/gen-contracts`.
- [x] 6.2 Add the four labels to `web/src/lib/labels.ts` and `extension/lib/labels.ts`, and a new group to `web/src/lib/filterSections.ts` — none of the existing eight fits a nurse. Run `svelte-check`.

## 7. Verification

- [x] 7.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, `golangci-lint run`; all clean.
- [x] 7.2 Re-run the mining pass and record the measured gain and each category's size in the change.
