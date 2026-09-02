## Why

After the IT-tail wave, **51 994 open postings across 1 772 title groups still
reach the search index with no role, and every one of them is an engineering
seat outside software**: Project Engineer (2 369), Quality Engineer (1 676),
`Инженер` (1 543), Field Service Engineer (1 045), Process Engineer (1 030),
`Инженер-технолог` (896), Automation Engineer (677), Maintenance Engineer
(598), Controls Engineer (404).

Half of it is Russian. `Инженер` and its qualified forms — `Инженер-технолог`,
`Инженер-энергетик`, `Инженер ПТО`, `Инженер по подготовке производства`,
`Инженер-электроник`, `Инженер по наладке и испытаниям` — carry no English
alias and resolve to nothing at all.

There is nowhere to put them today. `engineering_design` means draughting; a
Quality Engineer is not a draughtsman. The IT-tail wave deliberately left
`Automation Engineer` and `Application Engineer` unresolved for exactly this
reason: filing them under software would have been wrong for half of them.

## What Changes

- Add an `industrial_engineering` category: the engineering seats a factory,
  plant, utility or field-service organisation staffs — manufacturing, process,
  quality, maintenance, controls, commissioning, reliability, field service.
- Place it in the **non-technical** set, like `engineering_design`: filterable,
  but off the LLM enrichment and embedding budgets, since an IT job board is
  not where a process engineer looks for work.
- **BREAKING for `cmd/prune`'s internals:** turn the hard-coded
  `category != "engineering_design"` exception in `isBusinessCategory` into a
  named vocabulary set. A non-technical category that names a CRAFT must never
  be deletable as back-office work, and one hard-coded string cannot express a
  set of two. No behaviour changes for `engineering_design`.
- Resolve the Russian `инженер` family, including the qualified forms that name
  another discipline: `Инженер-проектировщик` goes to `engineering_design`,
  `Инженер по защите информации` to `security`.
- Declare the IT lookalikes the bare `engineer` alias would otherwise sweep:
  `IT Engineer`, `Database Engineer`, `Business Intelligence Engineer`,
  `Electronics Engineer`.
- Add named roles for the crafts: `project_engineer`, `quality_engineer`,
  `process_engineer`, `maintenance_engineer`, `controls_engineer`,
  `automation_engineer`, `field_service_engineer`, `industrial_engineer`.

Technicians, operators and assemblers are NOT in scope. "Service Technician"
(2 757) and "Assembler" (703) are a different facet from "Process Engineer",
and they belong with the trades wave.

## Capabilities

### New Capabilities
- `industrial-engineering-taxonomy`: which titles resolve to
  `industrial_engineering`, why it is non-technical yet undeletable, the
  Russian `инженер` family and the qualified forms that leave it, and the named
  roles the crafts expose.

### Modified Capabilities
- `catalog-pruning`: the business-category rule currently spares exactly one
  non-technical category by name. It must spare the set of non-technical
  categories that name a craft, so a second one cannot be added without the
  protection.

## Impact

- `internal/dict/vocab` — `CategoryValues`, `NonTechCategories`, and a new set
  naming the non-technical categories that are crafts rather than business
  work.
- `cmd/prune/rule.go` — `isBusinessCategory` reads the set instead of one
  hard-coded string.
- `internal/dict/classify` — the title `categoryTable`; the bare `engineer` and
  `инженер` aliases sit at the bottom, under every discipline that names
  itself.
- `internal/dict/roletag` — `categoryNoun` gains the category (required: it
  derives a bare and a graded role for every category), plus the new named
  roles.
- `cmd/gen-contracts` output, `web/src/lib/labels.ts`,
  `web/src/lib/filterSections.ts` and `extension/lib/labels.ts` — a category
  absent from the section map is unselectable, and the extension's map is
  exhaustive by contract.
- Rollout: `backfill-derive` at `BACKFILL_CONCURRENCY=2-3` in a quiet window,
  then a plain `make reindex`.
- Cost: none. The category is non-technical, so these postings stay off the
  enrichment and embedding queues.
