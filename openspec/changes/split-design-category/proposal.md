## Why

The `design` category conflates two unrelated crafts. On prod it holds 39 554 jobs
whose top skills are `figma` (6 639) **and** `autocad` (5 684), `revit` (2 846),
`verilog` (1 362), `tcl` (951) — a UX designer, a mining-equipment mechanical
engineer, and a VLSI physical-design engineer share one facet. The cause is a
single alias, `{"design", "design"}` in `internal/classify/dictionaries.go`, which
matches the word `design` in any "… Design Engineer" title. A 400-title sample
from inside `category=design` measures 24% explicit engineering-design titles, 33%
product/visual design, and 43% ambiguous (mostly engineering too).

Coverage of the design craft itself is thin at the same time: `roletag` carries six
named design roles, so 85% of the sampled titles fall through to the bare
"Designer" role, and the skill dictionary knows only `figma`, `photoshop`,
`premiere-pro`, `information-architecture`, `wcag`, `miro` — no `illustrator`,
`framer`, `webflow`, `prototyping`, `design-systems`, `user-research`.

## What Changes

- Add a `engineering_design` category to `vocab.CategoryValues` for engineering
  draughting/design work (mechanical, electrical, civil, chip), and place it in
  `vocab.NonTechCategories` — it is surfaced as a facet but kept out of the LLM
  enrichment and embedding budgets, like `marketing`/`sales`. **BREAKING** for
  consumers that assume `category=design` means product design: those jobs move
  category, and their `is_tech` flips from `true` to `false`.
- Route the engineering-design title aliases (`mechanical design engineer`,
  `electrical design engineer`, `civil design engineer`, `piping designer`,
  `pcb design`, `physical design engineer`, …) — and the bare `design engineer`,
  whose prod population is overwhelmingly mechanical — to `engineering_design`,
  ordered **before** the bare `design` alias. Product hybrids keep the `design`
  category only through explicit markers (`product design engineer`,
  `design systems engineer`, `ui engineer`).
- Extend `roletag` with the design roles the catalogue actually posts
  (`visual_designer`, `brand_designer`, `motion_designer`, `web_designer`,
  `ux_researcher`, `art_director`, `creative_director`, `design_ops`,
  `industrial_designer`, `design_engineer`) and with engineering-design roles
  (`mechanical_designer`, `electrical_designer`, `civil_designer`, `pcb_designer`,
  `chip_designer`), plus a role noun for the new category.
- Extend `skilltag` with the design tool and practice vocabulary (~25 canonicals:
  `illustrator`, `indesign`, `after-effects`, `framer`, `webflow`, `invision`,
  `blender`, `prototyping`, `wireframing`, `design-systems`, `user-research`,
  `usability-testing`, `interaction-design`, `design-thinking`, `typography`,
  `accessibility`, `motion-graphics`, …) and the CAD/EDA stack the engineering
  side needs (`solidworks`, `catia`, `creo`, `sketchup`, `altium`, `ansys`, …).
- Deliberately excluded: the homonym aliases `sketch`, `principle`, `eagle`, and a
  bare `maya` — in long prose they fire on ordinary English and on unrelated
  products, and precision beats recall in these dictionaries.

## Capabilities

### New Capabilities
- `design-taxonomy`: the split between product/visual design and engineering
  design — which category each title resolves to, which named roles the two sides
  expose, and the curated design/CAD skill vocabulary.

### Modified Capabilities
- `tech-classification`: the known non-technical category set gains
  `engineering_design`, so an engineering-design title derives `is_tech=false`
  instead of the `true` it currently inherits from `design`.

## Impact

- `internal/vocab/vocab.go` — `CategoryValues` + `NonTechCategories` (a partition
  test forces the membership choice).
- `internal/classify/dictionaries.go` — new aliases, ordered before bare `design`.
- `internal/roletag/roletag.go` — `categoryNoun` entry + named roles.
- `internal/skilltag/dictionaries.go` — design and CAD/EDA canonicals.
- `cmd/gen-contracts` output (`web/src/lib/generated/contracts.ts`) and the
  `CATEGORY_LABELS` maps in `web/src/lib/labels.ts` and `web/src/lib/insights.ts`.
- No DB migration: `jobs.category` carries no CHECK constraint.
- Post-deploy: `cmd/backfill-derive` re-derives category/skills/is_tech, then
  `make reindex` — the dictionary change does not reach existing jobs otherwise,
  and `is_tech` is not part of `content_hash`, so an incremental index pass would
  not pick the flip up.
- Enrichment/embedding queues shrink: engineering-design jobs stop being enqueued.
