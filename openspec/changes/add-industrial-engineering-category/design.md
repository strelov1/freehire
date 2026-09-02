## Context

After the IT-tail wave, 51 994 open postings across 1 772 title groups still
reach the index with no role, and the whole residue is one shape: engineering
seats outside software. There is nowhere to file them. `engineering_design`
means draughting — mechanical, electrical, civil, BIM — and a Quality Engineer
is not a draughtsman.

Roughly half is Russian, and none of it carries an English alias: `Инженер`
(1 543), `Инженер-технолог` (896+220), `Инженер по подготовке производства`
(453), `Инженер-энергетик` (411), `Инженер ПТО` (399), `Инженер-электроник`
(351), `Инженер по наладке и испытаниям` (286).

Two constraints shape the design:

- `vocab_test.go` asserts `TechCategories`, `NonTechCategories` and `{"other"}`
  partition `CategoryValues` exactly, so a new value must join one of the two.
- `cmd/prune/rule.go`'s `isBusinessCategory` deletes every `NonTechCategories`
  member except one, named inline: `category != "engineering_design"`.

## Goals / Non-Goals

**Goals:**

- One category for the engineering seats outside software.
- The Russian `инженер` family, including the qualified forms that leave it.
- The prune protection generalised from one inline name to a vocabulary set.
- Named roles for the crafts.

**Non-Goals:**

- **Technicians, operators and assemblers.** "Service Technician" (2 757),
  "Assembler" (703) and "Operator" (691) are a different facet from "Process
  Engineer" — different qualification, different search. They belong with the
  trades wave.
- **No sub-disciplines.** Manufacturing, process, quality, maintenance and
  controls are ONE category with named roles on top, not five categories. The
  role facet is where that granularity belongs; splitting the category would
  repeat the mistake the role facet exists to avoid.
- No description signal.

## Decisions

### The category is non-technical, and that is what makes the prune work necessary

An IT job board is not where a process engineer looks for work, so these
postings should not draw LLM enrichment or embedding budget — the same
reasoning `engineering_design` carries. But non-technical is also what
`cmd/prune`'s business rule deletes, and that rule spares exactly one category
by name.

Two categories cannot be expressed by one inline string, so the exception
becomes a named set in `vocab`. It lives in the vocabulary and not in prune
because it states what a category MEANS — "non-technical because the craft is
outside IT, not because the posting is back-office" — and a test asserts it is a
subset of `NonTechCategories`. Left as prune's private business, the next craft
category would be added to the vocabulary by someone who never opens
`cmd/prune`, and would become deletable in silence.

### Bare English `engineer` was tried and REJECTED by the existing suite

689 open postings spell it exactly, and the plan was to declare it last, below
every discipline that names itself. The suite refused it, and the refusal is
right on two counts:

- `Product Engineer`, `Growth Engineer`, `Staff Engineer`, `Lead Senior
  Engineer` and `Developer Onboarding Engineer` are each pinned to NO category
  ON PURPOSE — the word before "engineer" is what decides, and those words are
  ambiguous. One bare alias overrides every one of those decisions at once.
- `Categories()`, the multi-category path a CV profile reads, returns EVERY
  matching alias rather than the strongest. A bare "engineer" appends this
  category to "Senior Backend Engineer" and to every other engineering title in
  the catalogue.

`Reliability Engineer` (270) went the same way: the suite already pins it to no
category because mechanical reliability and site reliability share the phrase.

The Russian bare `инженер` stays. It does not carry the second problem to the
same degree — far fewer resolved titles here are headed by it — and its
qualified forms are declared above, so `Parse` is correct for every spelling
the catalogue actually contains.

### Bare `инженер` is declared LAST

Every software, data, security and infrastructure discipline names itself in
its own title, and all those aliases are declared above. What reaches the
bottom names no discipline of its own, and in a broad multi-industry ATS crawl
that residue is industrial. This is the same argument the IT wave made for bare
`systems engineer`, pointed the other way: there, the qualified industrial
spellings were declared blind ABOVE the bare alias; here, the qualified IT
spellings are.

Four IT titles have to be named explicitly because nothing else would catch
them: `IT Engineer` (158), `Database Engineer` (111), `Business Intelligence
Engineer` (111) and `Electronics Engineer` (122) — the last to `hardware`,
where the rest of electronics already lives.

### Two Russian titles leave the family by name

`Инженер-проектировщик` (236) is a draughtsman: it goes to
`engineering_design`. `Инженер по защите информации` (195) is an
information-security engineer: it goes to `security`. Both are declared above
the bare `инженер`, which would otherwise claim them — the same ordering the
English side uses.

### `Automation Engineer` and `Application Engineer` land here

Both were deliberately left unresolved by the IT wave for want of a home. In
this catalogue they read industrial: `Automation Engineer` (677) sits beside
`Controls Engineer` (404), and a QA automation engineer's title already carries
"QA", which resolves above. `Field Application Engineer` (126) is the
semiconductor pre-sales title and goes to `solutions_engineering` instead; the
bare `Application Engineer` (444) stays industrial.

## Risks / Trade-offs

- **A bare alias sweeps a discipline that should have kept itself** → declared
  last, with a regression test per software/data/security discipline asserting
  it is unchanged. This is the same guard the creative wave adopted after
  review found its craft aliases taking marketing rows.
- **The prune refactor changes deletion behaviour** → it only ADDS a spared
  category; `engineering_design` keeps its exact behaviour, and a test pins
  both the sparing and that the back-office categories are still deletable.
- **`Инженер` is broad** → it is broad in the source data too, and the
  qualified forms that name another discipline are routed by name. What remains
  is genuinely "some engineer at an industrial employer", which is exactly what
  the category says.
- **Re-measurable** → after rollout, `industrial_engineering` should land near
  the 52 000 measured here. Materially more means a bare alias took something.

## Migration Plan

1. Merge and deploy.
2. `backfill-derive` with `BACKFILL_CONCURRENCY=2-3` in a quiet window
   (02:00–05:00 UTC). At 6 it degraded the site while the worker's own log
   stayed smooth, so health is measured with `curl`, not from the log.
3. Stop `freehire-reindexw.timer`, rebuild, start the timer. No
   `REINDEX_DEDUP`.
4. Read the live `category` facet for `industrial_engineering` and compare
   against the measurement below.

### The measured result

Re-running the mining pass over the same dump with this change applied:

| | open postings | share |
|---|---|---|
| unroled before (with the IT wave merged) | 913 762 | 46.3% |
| unroled after | 891 574 | 45.2% |
| **gain** | **22 188** | |
| landing in `industrial_engineering` | 39 385 | |

The 39 385 is below the 51 994 the mining pass sized, and the difference is
the bare `engineer` alias and `reliability engineer` — both dropped once the
existing suite showed what they would have overridden. Sizing a cluster and
shipping a cluster are not the same number, and the gap between them is where
the deliberate non-decisions live.

Cumulative across both waves, from the original baseline: 935 604 → 891 574,
**44 030** open postings that now carry a role.

Rollback: revert the binary. Stored categories stay until the next
`backfill-derive` and are inert — every consumer reads the vocabulary from Go.
A reverted binary also restores prune's single-name exception, which is safe:
the category it protected is unchanged.

## Open Questions

None.
