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
  draughting/design work (mechanical, electrical, civil, architectural/BIM), and place it in
  `vocab.NonTechCategories` — it is surfaced as a facet but kept out of the LLM
  enrichment and embedding budgets, like `marketing`/`sales`. **BREAKING** for
  consumers that assume `category=design` means product design: those jobs move
  category, and their `is_tech` flips from `true` to `false`.
- Route the engineering-design title aliases (`mechanical designer`,
  `electrical designer`, `civil designer`, `piping designer`, `architectural designer`,
  `bim designer`, `drafter`, …) — and the bare `design engineer`,
  whose prod population is overwhelmingly mechanical — to `engineering_design`,
  ordered **before** the bare `design` alias. Product hybrids keep the `design`
  category only through explicit markers (`product design engineer`,
  `design systems engineer`, `ui engineer`).
- Extend `roletag` with the design roles the catalogue actually posts
  (`visual_designer`, `brand_designer`, `motion_designer`, `web_designer`,
  `ux_researcher`, `art_director`, `creative_director`, `design_ops`,
  `industrial_designer`, `design_engineer`) and with engineering-design roles
  (`mechanical_designer`, `electrical_designer`, `civil_designer`, and — inside the
  `hardware` category — `pcb_designer` and `chip_designer`), plus a role noun for the
  new category.
- Extend `skilltag` with the design tool and practice vocabulary (`illustrator`,
  `indesign`, `after-effects`, `adobe-xd`, `webflow`, `invision`, `zeplin`, `lottie`,
  `prototyping`, `wireframing`, `user-research`, `usability-testing`,
  `interaction-design`, `design-thinking`, `typography`, `motion-design`,
  `motion-graphics`, `user-flows`, …) and the CAD/EDA stack the engineering side needs
  (`solidworks`, `catia`, `creo`, `sketchup`, `altium`, `kicad`, `ansys`,
  `autodesk-inventor`, `fusion-360`, `3ds-max`, `civil-3d`, `cadence-virtuoso`, …).
- Keep every deletion path off the new category. A resolved `engineering_design`
  vetoes `ConfirmedNonTech`, which covers the ingest catalogue filter, the liveness
  refresh and prune's title rule; prune's business rule reads the category set
  directly, so it gets its own exclusion (`isBusinessCategory`). While these titles
  lived in `design`, `TechCategories` supplied all of that for free — and without it an
  "HVAC Designer", an alias this change ADDS, was rejected at ingest and hard-deletable.
- Mask the titles where "design" names no craft at all (`software design engineer`)
  with a blind table entry, so they resolve to no category instead of being filed as
  draughting — and route the ones that DO have a category (`cloud design engineer` →
  devops, `design engineer in test` → qa, `service`/`sound`/`game design engineer` →
  design). Silicon design goes to the existing `hardware`, not to the new category.
- Homonyms are handled by degree, not by a blanket rule. Behind the existing
  corroboration gate (`ambiguousWords`), so they resolve only next to a strong token:
  `sketch`, `maya`, `lottie`, `blender`, `illustrator`, `canva`, `typography`,
  `wireframes`, `prototyping`, `invision`, `accessibility`. Excluded outright:
  `principle`, `eagle`, a bare `nx`, `framer` (the carpentry trade AND Framer Motion),
  `spline` (the splined shafts of the mechanical population being split out), and the
  phrases `design system(s)`, `visual design`, `solid edge` and bare `after effects` —
  each is ordinary prose, and a phrase always matches STRONG, which would also lift
  the gate off every weak word beside it. `creo` resolves only through
  `ptc creo`/`creo parametric` ("creo" is Spanish for "I think").

## Capabilities

### New Capabilities
- `design-taxonomy`: the split between product/visual design and engineering
  design — which category each title resolves to, which named roles the two sides
  expose, and the curated design/CAD skill vocabulary.

### Modified Capabilities
- `tech-classification`: the known non-technical category set gains
  `engineering_design`, so an engineering-design title derives `is_tech=false`
  instead of the `true` it currently inherits from `design`. Silicon design keeps
  `is_tech=true` — it moves to `hardware`, not to the new category.

## Impact

- `internal/vocab/vocab.go` — `CategoryValues` + `NonTechCategories` (a partition
  test forces the membership choice).
- `internal/classify/dictionaries.go` — new aliases, ordered before bare `design`,
  plus the `categoryNone` sentinel for the blind ones.
- `internal/classify/classify.go` — `matchCategory` translates the sentinel to ""; the
  same translation guards `Categories()` (the CV path) and `CategoryAliases()` (the
  generated web contracts).
- `internal/classify/tech.go` — `software design engineer` joins the tech-title terms,
  so a masked software title still reads as technical.
- `internal/classify/nontech.go` — `ConfirmedNonTech` gains the category veto, which
  covers three deletion callers at once (ingest filter, liveness refresh, prune title
  rule).
- `cmd/prune/rule.go` — `isBusinessCategory` keeps the fourth path, the business rule,
  off the new category.
- `internal/roletag/roletag.go` — `categoryNoun` entry + named roles.
- `internal/skilltag/dictionaries.go` — design and CAD/EDA canonicals.
- `cmd/gen-contracts` output (`web/src/lib/generated/contracts.ts`), the full
  `CATEGORY_LABELS` map in `web/src/lib/insights.ts`, and `CATEGORY_GROUP` in
  `web/src/lib/filterSections.ts` — the last one is type-checked exhaustively, and
  without it the new value is in no section of the Specialization pane.
  `web/src/lib/labels.ts` needs nothing: that map holds only the values `humanize()`
  gets wrong.
- No DB migration: `jobs.category` carries no CHECK constraint.
- Post-deploy: `cmd/backfill-derive` re-derives category/skills/is_tech, then
  `make reindex` — the dictionary change does not reach existing jobs otherwise,
  and `is_tech` is not part of `content_hash`, so an incremental index pass would
  not pick the flip up.
- Enrichment/embedding queues shrink: engineering-design jobs stop being enqueued.
