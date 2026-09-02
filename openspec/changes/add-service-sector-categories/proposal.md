## Why

Three waves in, 766 437 open postings still reach the search index with no
role. Four clusters account for the last coherent part of it:

| cluster | open postings |
|---|---|
| logistics | 47 551 |
| education | 23 226 |
| personal services | 6 851 |
| administration | 4 363 |

Administration is small only because the previous wave took the grocery clerks
into `retail`, where they belonged; what is left is genuinely office work —
`Администратор` (1 132), `Секретарь` and its qualified forms, Data Entry.

The same mining pass surfaced a **defect in the wave that just shipped**:
`wordmatch` matches whole words with no morphology, so a singular alias does
not reach a plural title. `Automotive Mechanics` (1 292), `AUTOMOTIVE TIRE
TECHNICIANS` (1 248) and `Automotive Alignment Technicians` (490) all still
resolve to nothing, because the aliases were written singular. `Optometrist`
(1 121) and a bare `Host` (3 446) were missed outright.

## What Changes

- Add four categories: `logistics`, `education`, `personal_services`,
  `administration`.
- All four are **non-technical** and, like the consumer industries before them,
  are NOT added to the craft set `cmd/prune` spares — so deletion behaviour
  stays exactly as it is.
- Fix the plural gap in the shipped trades vocabulary: `mechanics`,
  `technicians` and the qualified automotive spellings.
- Add the titles the previous wave missed: `optometrist` and the bare `host`.
- Add the Russian service vocabularies: `водитель`, `курьер`, `кладовщик`,
  `экспедитор`, `сборщик заказов`, `грузчик`; `педагог`, `воспитатель`,
  `преподаватель`, `методист`; `секретарь`, `делопроизводитель`; `парикмахер`,
  `уборщик`, `охранник`, `сиделка`.
- Add the Russian building-maintenance workers — `Рабочий по комплексному
  обслуживанию и ремонту зданий` (4 120) and `Рабочий по благоустройству`
  (1 136) — to `skilled_trades`, where the rest of that work already sits.
- Add named roles for the seats carried in volume.

## Capabilities

### New Capabilities
- `service-sector-taxonomy`: which titles resolve to `logistics`, `education`,
  `personal_services` and `administration`, the Russian vocabularies inside
  them, and the named roles they expose.

### Modified Capabilities
- `consumer-industry-taxonomy`: the trades vocabulary must reach the PLURAL
  spellings of its own titles. `wordmatch` has no morphology, and the shipped
  aliases are singular, so three of the largest automotive titles in the
  catalogue still resolve to nothing.

## Impact

- `internal/dict/vocab` — `CategoryValues` and `NonTechCategories`. NOT
  `NonTechCraftCategories`.
- `internal/dict/classify` — the title `categoryTable`; the bare `водитель` is
  the hazard here, since `Делопроизводитель` ends in it.
- `internal/dict/roletag` — `categoryNoun` for all four plus the named roles.
- `cmd/gen-contracts` output, `web/src/lib/labels.ts`,
  `web/src/lib/filterSections.ts` and `extension/lib/labels.ts`.
- Rollout: folded into the `backfill-derive` already scheduled for 02:12 UTC —
  if this lands before it, one pass covers all four waves.
- Cost: none. All four are non-technical.
