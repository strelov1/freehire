## Context

`internal/classify` resolves `jobs.category` from the title by an ordered
alias table, first whole-word match wins. The bare alias `{"design", "design"}`
therefore claims every "… Design Engineer" title. Measured on prod:
`category=design` holds 39 554 jobs whose skill facet mixes `figma` (6 639) with
`autocad` (5 684), `revit` (2 846), `verilog` (1 362) and `tcl` (951); a phrase
search for `"design engineer"` returns 8 773 jobs, 7 748 of them in `design`.

Three dictionaries are in play, all dict-only and all fed by the same doctrine
(never guess):

- `internal/classify` — title → seniority + category (ordered table, first match).
- `internal/roletag` — (seniority, category, title) → role slugs. Named aliases are
  sorted longest-first and **at most one** named role is emitted (`break` after the
  first match), so a longer, more specific alias always beats a shorter one nested
  inside it.
- `internal/skilltag` — description → skill canonicals. Single tokens live in
  `wordAliases`; multi-word and punctuated terms in `engineeringPhraseAliases` /
  `professionalPhraseAliases`. `ambiguousWords` marks weak single tokens that are
  only tagged when a strong tech token corroborates them in the same text.

`vocab.CategoryValues` is the shared enum; `vocab_test.go` asserts that
`TechCategories`, `NonTechCategories` and `{"other"}` partition it exactly, so a new
value must be assigned to one of the two sets. `jobs.category` has no CHECK
constraint, so no migration is needed. The archived change
`2026-07-01-add-ai-engineering-category` is the precedent for the full edit surface.

## Goals / Non-Goals

**Goals:**

- `category=design` means product/visual/experience design and nothing else.
- Engineering draughting work is filterable in its own right, not hidden.
- A design title resolves to a specific role instead of the bare "Designer"
  (currently 85% of sampled design titles fall through).
- The design craft's tools and practices are taggable skills; so is the CAD/EDA
  stack the engineering side states.

**Non-Goals:**

- Reclassifying engineering titles that do **not** currently land in `design`.
  A bare "Mechanical Engineer" or "Civil Engineer" stays unresolved — it is not
  polluting the design facet, and pulling the whole non-IT engineering world into
  the taxonomy is a separate, much larger question.
- A new vocab value for silicon design. It routes to the existing `hardware`
  category instead; only one value is added by this change.
- Touching `NonTechFromDescription` (the description-level non-tech detector).
  The title carries the signal for these roles.
- Changing how `prune`'s board shield reads skills (see Risks).

## Decisions

### 1. One new category, `engineering_design`, in `NonTechCategories`

Alternatives considered: (a) route the engineering titles into existing
`hardware`/`other` — no vocab growth, but `other` is a landfill for 20k+ jobs; (b) two
values, `chip_design` (tech) + `engineering_design` (non-tech) — accurate, but a new
vocab value for a distinction `hardware` already expresses.

Chosen: one value, non-technical. An engineer who designs mining wear liners is
not the audience of an IT job board, and `NonTechCategories` is exactly the
"surface it as a facet, keep it off the LLM budget" bucket (it gates both
`enrichment_outbox` and `semantic_outbox` enqueues). Consequence: `is_tech` flips
`true → false` for these jobs, which the `tech-classification` delta records.

Silicon is the exception, and it goes the (a) way: the whole family — `pcb design`,
`pcb layout`, `physical`/`analog`/`rtl`/`mixed signal`/`digital`/`dft design engineer`,
`vlsi`/`chip`/`asic`/`soc`/`ic`/`semiconductor design` — resolves to the existing
**`hardware`** category. That was not the first cut; they were filed as draughting, and
review showed the line was incoherent, because `hardware design engineer` and
`fpga design engineer` resolve to `hardware` through earlier aliases. Their own
colleagues would have landed in a non-technical facet and lost the enrichment and
embeddings they have today. No new vocab value is needed for it.

**Membership in `NonTechCategories` is not only an `is_tech` statement — three code
paths DELETE on it**, and all three had to be taught about this category:

- the ingest catalogue filter and the liveness refresh (`internal/pipeline`), and
- `prune`'s title rule — all three via `classify.ConfirmedNonTech`, whose
  `hasTechEvidence` argument is computed from `TechCategories`;
- `prune`'s business rule (`cmd/prune/rule.go`), which reads `NonTechCategories`
  **directly** and so bypasses that veto entirely.

Leaving `design` removed the veto from a population the non-tech title dictionary
already anchors by trade name (`hvac`, `sheet metal`, `machinist`), so an "HVAC
Designer" was turned away at ingest and hard-deleted by prune — the alias this change
adds, deleting the jobs it was written to classify. The veto is restored inside
`ConfirmedNonTech`, which covers the first three callers in one place and leaves
`is_tech` untouched; the business rule gets its own `isBusinessCategory` helper, since
draughting is not a business role at a software employer. Both are scoped to this one
category: widening either to "any resolved category" would change the catalogue filter
for `marketing`/`sales`/`support`, which is outside this change.


### 2. Bare `design engineer` → `engineering_design`, product hybrids by explicit marker

The bare title is genuinely ambiguous in the industry, but not in our data: the
sampled bare "Design Engineer" titles are mechanical/industrial (the first prod hit
is mining equipment). Routing it to the engineering side is a statement about the
catalogue, not a guess about the word.

The product-engineering hybrid is recognized only through markers that cannot be
read any other way: `product design engineer`, `design systems engineer`,
`design system engineer`, `ux design engineer`, `ui design engineer`,
`ui/ux design engineer`, `web design engineer`, `design engineer, product`. Ordered
ahead of the bare alias, so the specific title wins the first-match table. A bare
"UI Engineer"/"UX Engineer" needs no marker — it never reaches the block, since the
pre-existing `ui`/`ux` aliases below already resolve it to `design`.

Alias order inside the table (engineering block placed immediately **before** the
existing `designer`/`design` entries, itself preceded by the marker block):

1. titles that name another craft: `network design engineer` → `network_engineering`,
   `cloud design engineer` → `devops`, `solution(s) design engineer` →
   `solutions_engineering`, and the silicon family (`pcb design`, `pcb layout`,
   `physical`/`analog`/`rtl`/`mixed signal`/`digital`/`dft design engineer`,
   `vlsi`/`chip`/`asic`/`soc`/`ic`/`semiconductor design`) → `hardware`
2. product-side markers → `design`: `product design engineer`,
   `design system(s) engineer`, `ux`/`ui`/`ui-ux`/`web design engineer`,
   `design engineer, product`, and the design disciplines of their own
   (`service`/`experience`/`sound`/`game design engineer`)
3. `engineering_design`: the `…designer` nouns (`mechanical`, `electrical`, `civil`,
   `structural`, `piping`, `plumbing`, `hvac`, `cad`, `tool`, `mold`, `die`,
   `architectural`, `bim`, `revit`), the draughting professions (`drafter`,
   `draftsman`, `draughtsman`, `design drafter`, `design draftsman`,
   `design technician`, `bim coordinator`, `bim modeler`, `конструктор`), and the
   closing bare `design engineer`
4. existing `designer` / `design` / `ux` / `ui` → `design`

No bare `layout designer`: magazine and print layout is the product-design craft and
the phrase names both trades.

Group 1 has to be exhaustive within each family. Whatever a family omits falls
through to the closing bare alias and lands in draughting — that is how the first cut
sent `ASIC`/`SoC`/`IC design engineer` to a non-technical facet while their
`hardware design engineer` colleagues stayed technical.

Every qualified `<discipline> design engineer` form is left OUT of that list: the
closing bare alias already resolves it to the same category, so an explicit entry
would be dead weight in an ordered table (where a redundant row reads as if the
order mattered). Only the titles the bare alias cannot see are listed.

`hardware design engineer` and `firmware design engineer` need no entry either: the
`hardware`/`embedded` aliases already precede the design block.

### 2b. A blind table entry for the titles that name no category at all

"Software Design Engineer" is software engineering — "design" qualifies what is
engineered. There is no honest category for it in this vocabulary (a bare "Staff
Software Engineer" resolves none either), so routing it anywhere is worse than
emitting nothing: as draughting it left the technical catalogue entirely, losing
enrichment, embeddings and its `is_tech`.

The first attempt copied `gradeBlindPhrases` — mask the phrase before the match — and
that was wrong twice over, which review caught:

- cutting the span exposes whatever alias sits FURTHER DOWN the table, and the aliases
  below the design block are almost exactly the business categories. "Software Design
  Engineer - Sales Tools" resolved to `sales` and lost its enrichment;
- a mask is boundary-blind where every matcher here is boundary-aware, so
  "Software Design Engineering Manager" became `"  ing manager"` → `management`.

It also over-reached: `systems design engineer` was on the mask list, which blanked the
category of every "HVAC / Mechanical Systems Design Engineer" — and with it the
placement that vetoes deletion, so those postings became rejectable at ingest and
hard-deletable by prune. Exactly the defect §1 exists to prevent.

The shipped shape is a table entry carrying the `categoryNone` sentinel. It wins the
first-match walk like any other alias, so nothing below it is exposed; `matchCategory`
translates the sentinel to `""`. Three exits need the same translation, and each has a
test: `Parse`, `Categories` (the CV path, which would otherwise put `-` in a profile)
and `CategoryAliases` (which feeds the generated web contracts, where `-` would ship as
a pickable value). `is_tech` comes from the tech-title detector, which gains
`software design engineer` as a term — it is not adjacent to the existing
`software engineer`, so `wordmatch` could not see it.

Only phrases with no better category belong there, and they must be narrow. Where a
category exists, route the title to it: `cloud design engineer` → `devops`,
`solution(s) design engineer` → `solutions_engineering`,
`(software) design engineer in test` → `qa`,
`service`/`experience`/`sound`/`game design engineer` → `design`.

### 3. Named roles on both sides; longest-alias-first does the disambiguation

Because `roletag` sorts named aliases by length and emits at most one, adding both
`design_engineer` (alias `design engineer`) and `mechanical_designer` (aliases
`mechanical design engineer`, `mechanical designer`) is safe: the longer alias wins
for a qualified title. `design_engineer` is therefore honest as the *unqualified*
role, and the specific engineering roles shadow it exactly when they should.

Product roles: `visual_designer`, `brand_designer`, `motion_designer`,
`web_designer`, `industrial_designer`, `ux_researcher` (aliases `ux researcher`,
`user researcher`, `ux research`), `art_director`, `creative_director`,
`design_ops` (`design ops`, `designops`, `design operations`), `design_engineer`.
`design_systems_engineer` is folded into `design_engineer` rather than given its own
slug — one alias family, one picker entry.

Engineering roles: `mechanical_designer`, `electrical_designer`, `civil_designer`,
`pcb_designer` and `chip_designer` (`physical design engineer`,
`vlsi design engineer`, `rtl design engineer`, `analog design engineer`) — the last
two name specializations INSIDE `hardware`, where the bare "Hardware Engineer" role
is otherwise all a silicon title gets.

`categoryNoun["engineering_design"] = "Engineering Designer"` gives the new category
its bare role and seniority composites, matching every other decomposable category.
`art_director`/`creative_director`/`design_ops` are directorial titles that already
state their level — they join `nonGradeable` so we do not mint
"Senior Creative Director".

### 4. Skills: add the design and CAD vocabulary, route homonyms through `ambiguousWords`

Single tokens (`wordAliases`): `indesign`, `webflow`, `invision`, `zeplin`,
`protopie`, `figjam`, `prototyping`, `wireframing`, `a11y`, `solidworks`, `catia`,
`sketchup`, `altium`, `kicad`, `ansys` — plus nine tokens behind the gate below.

Phrases (`engineeringPhraseAliases`): `after effects`, `adobe xd`,
`design thinking`, `user research`, `ux research`,
`usability testing`, `interaction design`, `visual design`, `motion design`,
`motion graphics`, `user flows`, `3ds max`, `fusion 360`, `civil 3d`, `solid edge`,
`autodesk inventor`, `siemens nx`, `ptc creo`, `creo parametric`,
`cadence virtuoso`. `adobe illustrator`/`adobe indesign` need no phrase — the word
pass already sees the product name inside them.

Homonyms are handled by degree. Behind the existing corroboration gate
(`wordAliases` **and** `ambiguousWords`, so they resolve only next to a strong token):
`sketch` ("sketch out ideas"), `maya` and `lottie` (people's names), `blender` (a line
cook's appliance), `illustrator` (also the occupation), `canva` (a store's poster
tool), `typography` ("typography of the shelf labels"), `wireframes` (CAD prose) and
`accessibility` (a broad concept word, and a ramp at the entrance).

That list is longer than the obvious homonyms because of a second-order effect: a
STRONG match lifts the gate off every weak word in the same text. Leaving `typography`
strong made the gate on `sketch` decorative — "Typography of the shelf labels …
sketch out ideas for displays" tagged both. A design word that is also ordinary prose
cannot be a corroborator.

Excluded outright, because the other sense dominates even in corroborated text:
`principle`, `eagle` (the bird; "eagle-eyed" is posting boilerplate), a bare `nx` (the
Nx JS monorepo tool), `framer` (the carpentry trade AND Framer Motion), `spline` (the
splined shafts of the mechanical population this split is about) and the phrase
`design system(s)` — that one is the ordinary verb reading of every backend posting
("you will design systems that scale"), and as a phrase it would always match strong.
`creo` is Spanish for "I think", so it resolves through its phrases only. Where a real
product is excluded, an unambiguous phrase keeps it reachable (`siemens nx`,
`ptc creo`); the design-systems practice stays reachable through the job title.

Note that two gated words do not corroborate each other — the gate demands a
*strong* token — so "Sculpting in Maya and Blender" tags neither. That is the
intended conservatism: a 3D posting names a strong tool too.

### 5. Where the new skill canonicals live w.r.t. `HasEngineering`

They go in the engineering tables (or `wordAliases`), like the existing `figma` /
`autocad`. That means `prune`'s board shield counts a design-only or CAD-only board
as "has posted something technical". That matches today's behaviour for `figma` and
`autocad` and keeps the shield conservative — a false keep is cheaper than a false
retire.

## Risks / Trade-offs

- **20k+ jobs flip `is_tech` to `false` and leave `category=design`.** → Intended,
  and recorded as a spec delta. Users filtering `category=design` see a smaller,
  correct facet; the engineering jobs remain findable under
  `category=engineering_design`. Announce in the changelog.
- **`is_tech` is not part of `content_hash`, so an incremental index pass will not
  carry the flip into Meilisearch.** → The migration plan runs `backfill-derive`
  followed by a full `make reindex`, never an incremental pass.
- **Enrichment/embedding queues drop these jobs; already-enriched ones keep their
  LLM payload.** → Harmless: served facets are dict-only, so the stale
  `enrichment.category` is never surfaced.
- **A homonym slips through corroboration** (a mechanical description that mentions
  "sketch" next to `autocad`). → Acceptable: the tag would be wrong but not
  misleading, and corroboration already gates the worst cases. Tests cover the
  uncorroborated prose.
- **Alias-order regressions**: inserting a block into an ordered table can shadow an
  existing category. → The RED test for each block asserts the neighbours
  (`hardware design engineer` → `hardware`, `content designer` →
  `technical_writing`, `ux writer` → `technical_writing`) still resolve as before.

## Migration Plan

1. Ship the code change (no DB migration — `jobs.category` has no CHECK).
2. Regenerate contracts (`cmd/gen-contracts`), add
   `engineering_design: 'Engineering Design'` to the full `CATEGORY_LABELS` map in
   `web/src/lib/insights.ts`, and map the value in `CATEGORY_GROUP`
   (`web/src/lib/filterSections.ts`) so it appears in the Specialization pane.
   Verify with `svelte-check`, not only lint+build: the group map is the one place
   the omission surfaces, and only as a type error.
3. Deploy.
4. On prod: `cmd/backfill-derive` (re-derives category, skills, is_tech,
   role_fingerprint), then `make reindex` — a full swap, and **not** stacked with
   `reindex-companies`.
5. Verify on the live API: `category=design` skill facet no longer shows
   `autocad`/`revit` in the top ranks (`verilog`/`tcl` move to `hardware`, not out of
   the technical catalogue), and `category=engineering_design`
   returns the expected population.

Rollback: revert the commit and re-run `backfill-derive` + `reindex`. The category
value would then be absent from the vocabulary while some rows still carry it —
harmless (an unknown facet value is served as-is and the web label map falls back to
`humanize`), and the backfill clears it.

## Open Questions

None. The three decisions the scope hinged on (new category, its tech membership,
the bare-title routing) were settled before this document was written.
