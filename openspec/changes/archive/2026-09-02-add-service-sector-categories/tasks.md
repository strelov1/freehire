## 1. Vocabulary

- [x] 1.1 Add `logistics`, `education`, `personal_services`, `administration` to `vocab.CategoryValues` and `vocab.NonTechCategories`, with a comment on each.
- [x] 1.2 Extend the craft-set test so it asserts none of the four is craft-protected, keeping deletion behaviour unchanged.

## 2. Fixes to the shipped vocabulary

- [x] 2.1 Add the PLURAL trade spellings — `mechanics`, `technicians`, `automotive mechanics`, `automotive technicians`, `alignment technicians` — with tests that both spellings resolve alike.
- [x] 2.2 Add `optometrist` → `healthcare` and bare `host` → `hospitality`, with a test that a hosting title is unaffected.
- [x] 2.3 Add the Russian building-maintenance workers → `skilled_trades`, with tests.

## 3. Logistics

- [x] 3.1 Add the English logistics vocabulary → `logistics`: delivery specialist/driver, commercial driver, CDL driver, driver, courier, warehouse associate/operator/supervisor, forklift operator, picker, packer, truck unloader, fulfillment associate, dispatcher, freight. With tests.
- [x] 3.2 Add the Russian logistics vocabulary → `logistics`: `водитель`, `курьер`, `кладовщик`, `экспедитор`, `грузчик`, `сборщик заказов`, `заведующий складом`. With the `Делопроизводитель` regression test asserting `administration`, NOT `logistics`.

## 4. Education, administration, personal services

- [x] 4.1 Add the education vocabulary → `education`: teacher, tutor, instructor, swim/chess instructor, preschool teacher, lecturer, professor; plus `педагог`, `воспитатель`, `преподаватель`, `методист`. With tests.
- [x] 4.2 Add the administration vocabulary → `administration`: receptionist, secretary, office manager, administrative assistant, data entry; plus `секретарь`, `делопроизводитель`, `администратор`. With tests.
- [x] 4.3 Add the personal-services vocabulary → `personal_services`: stylist, barber, esthetician, lifeguard, security guard, janitor, custodian, housekeeper, cleaner; plus `парикмахер`, `уборщик`, `охранник`, `сиделка`. With tests.

## 5. Named roles

- [x] 5.1 Add `categoryNoun` entries for all four.
- [x] 5.2 Add the named roles `driver`, `warehouse_associate`, `dispatcher`, `teacher`, `instructor`, `receptionist`, `stylist`, `security_guard`, `cleaner`, with tests that they do not steal from the earlier waves.

## 6. Contracts and the two apps

- [x] 6.1 Regenerate `web/src/lib/generated/contracts.ts`.
- [x] 6.2 Add the four labels to `web/src/lib/labels.ts` and `extension/lib/labels.ts`, and the section entries to `web/src/lib/filterSections.ts`. Run `svelte-check`.

## 7. Verification

- [x] 7.1 Run `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, `golangci-lint run`; all clean.
- [x] 7.2 Re-run the mining pass and record the measured gain and each category's size.
