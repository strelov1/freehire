# Occupational Safety (HSE) — a category of its own

Date: 2026-09-24
Status: approved, ready for an implementation plan

## The ask

A subscriber works in HSE, mostly oil & gas. The titles that name their profession
spell it a dozen ways — HSE, EHS, HSSE, QHSE, HSEQ, SHE, SHEQ, HSSE&SP, HSE&S, SHES,
plus the spelled-out "Health & Safety" and "Environmental Health and Safety" — and the
catalogue recognises none of them.

## What production says

Measured on 2026-09-24 against `jobs` on prod, restricted to `closed_at IS NULL AND
duplicate_of IS NULL`.

The acronyms alone (`hse|ehs|hsse|qhse|hseq|sheq|shes`) match ~6,200 live postings:

| category today        | count |
| --------------------- | ----- |
| (empty)               | 4,515 |
| management            | 1,454 |
| project_management    |    55 |
| industrial_engineering|    46 |
| support               |    35 |
| operations            |    19 |

The top titles are unambiguous: `EHS Manager` (255), `EHS Specialist` (208),
`HSE Manager` (158), `EHS Coordinator` (95), `HSE Specialist` (92), `HSE Officer` (87),
`HSE Advisor` (66), `HSEQ Advisor` (14), `QHSE Manager` (13), `HSSE Manager` (13).

Widening to the word `safety` adds several thousand more with an empty category:
`Safety Coordinator` (283), `Safety Specialist` (278), `Safety Officer` (130),
`Health & Safety Advisor` (30), `Environmental Health and Safety Specialist` (30).

Across all three word families (HSE acronyms + `safety` + `risk`) the live population
is ~37,000, of which **24,257 carry no category at all**.

Two failures, and they are different failures:

1. **4,515 acronym postings resolve nothing.** An empty category means the posting is
   unreachable from every facet. The subscriber cannot find it.
2. **1,454 resolve `management`** — not because anything understood them, but because
   `dictionaries.go:1047` carries `{"manager", "management"}` as a fall-through and the
   HSE blocks that would outrank it do not exist. `HSE Manager` currently files
   alongside `Sales Manager`.

Today the whole dictionary holds exactly one relevant line:
`dictionaries.go:1668` — `{"safety engineer", "industrial_engineering"}`.

## Why a category rather than a fold

Two national occupational classifications give occupational safety its own unit, and
neither files it under engineering:

- **O\*NET / SOC (US)** — minor group `19-5000 Occupational Health and Safety
  Specialists and Technicians`, split into `19-5011` (specialists) and `19-5012`
  (technicians), under *Life, Physical and Social Science*. Its reported-title list
  names EHS Officer, Safety Specialist, Risk Control Consultant and Industrial
  Hygienist directly.
- **ISCO-08 / ESCO (EU)** — unit group `2263 Environmental and occupational health and
  hygiene professionals`, under minor group 226 *Other Health Professionals*. The ESCO
  concept `2263.3` is literally `health and safety officer`.

LinkedIn's 26 job functions carry no HSE function, but that taxonomy is far coarser
than ours — it has no Customer Success function either. Our vocabulary runs ~45
categories, which is SOC-minor-group granularity, so the government classifications are
the relevant precedent.

Folding HSE into `industrial_engineering` was considered and rejected: both canonical
systems place it away from engineering, and the facet would then lie — an HSE Manager
is a compliance and controls role, not a plant engineer, and a subscriber selecting
"Industrial Engineering" would get a factory mixed with a safety department.

## Design

### 1. Name

Canonical key `occupational_safety`, after the SOC and ISCO group names.

User-facing label `Health & Safety (HSE)`. The acronym is carried in the label on
purpose: it is what the population calls itself, and it makes the filter findable by
the word the subscriber already types.

`health_safety` was rejected as a key — the catalogue already has `healthcare`, and the
two would be read as siblings.

### 2. Vocabulary placement — `internal/dict/vocab/vocab.go`

- `CategoryValues` — add `occupational_safety`.
- `NonTechCategories` — add it. It is a real craft, but not the IT work this catalogue
  serves, so it is filterable without spending LLM or embedding budget, exactly like
  `engineering_design` and `industrial_engineering`.
- `NonTechCraftCategories` — **add it**. This set is what `cmd/prune`'s business rule
  SUBTRACTS. Without membership, the moment an oil & gas or construction employer's
  board is retired, prune deletes that employer's entire catalogue — the failure mode
  the set was created to prevent.

A test already asserts the three sets partition `CategoryValues` exactly, and that
every `NonTechCraftCategories` member is also a `NonTechCategories` member. Both must
stay green.

### 3. The deletion veto — `internal/dict/classify/nontech.go`

`nontechTerms` already carries `"охрана труда"` and `"охране труда"` (line 182) — the
Russian name of this exact profession. `ConfirmedNonTech` is consulted by two DELETE
paths: the ingest catalogue filter and the prune title rule. So today a Russian HSE
title is turned away at ingest and hard-deleted from storage.

`ConfirmedNonTech` (line 203) must therefore veto on `occupational_safety` the way it
already vetoes on `engineering_design`, and for the identical reason: the term list and
the category describe the same trade from two sides, so a word match between them is
not the accidental kind the veto was built for.

Without this step the category is cosmetic — the postings never reach the facet.

### 4. Title dictionary — `internal/dict/classify/dictionaries.go`

A new block placed **above line 1047** (`{"manager", "management"}`). The table's order
is its precedence, and that single line is what currently claims 1,454 HSE postings.

Admitted aliases:

- Acronyms: `hse`, `ehs`, `hsse`, `qhse`, `hseq`, `sheq`, `shes`, `hsse&sp`, `hse&s`.
- Spelled out: `health and safety`, `health & safety`, `environmental health and
  safety`, `environmental health & safety`, `occupational health and safety`,
  `охрана труда`, `охране труда`.
- SOC reported titles: `safety officer`, `safety specialist`, `safety coordinator`,
  `safety advisor`, `safety supervisor`, `safety technician`, `safety director`,
  `industrial hygienist`, `risk control consultant`.
- `safety engineer` — moved here from `industrial_engineering` (line 1668).

Deliberately **not** admitted, each for a measured reason:

- **Bare `she`.** Live titles carry pronouns (`Engineer (she/her)`), and `she` is an
  ordinary English word. Only the qualified forms are admitted: `she manager`,
  `she officer`, `she advisor`, `she coordinator`, `she specialist`. This follows the
  precedent already set in this table for bare `security` and bare `mobile`.
- **Bare `risk`.** `Risk Analyst`, `Risk Manager` and `Credit Risk` are finance, not
  HSE, and prod carries 41,468 postings matching the word. Only `hse risk` and
  `safety risk` are admitted, plus SOC's `risk control consultant`.
- **Bare `safety`** as a standalone alias. The qualified title forms above cover the
  population; the bare word appears in `Patient Safety Attendant` (healthcare),
  `Campus Safety Officer` and `Public Safety Officer` (protective services) and
  `Food Safety` (manufacturing QA), which are other professions.

Every admitted alias should be sampled against live titles before it lands, the way
every other term in this table was admitted.

### 5. Serving surfaces

- `web/src/lib/labels.ts` — `CATEGORY_LABELS.occupational_safety = 'Health & Safety (HSE)'`.
- `extension/lib/labels.ts` — the same entry.
- `web/src/lib/filterSections.ts` — `CATEGORY_GROUP.occupational_safety = 'Quality & Security'`.
  That group already holds `qa` and `security`; the QHSE and HSEQ acronyms bundle
  Quality with HSE themselves, so the grouping matches what the profession calls itself.
  A category with no group is generated into the contracts but unreachable in the
  picker, so this entry is load-bearing, not decorative.
- `cmd/gen-contracts` — regenerate `web/src/lib/generated/contracts.ts`.

`CATEGORY_GROUP` is typed `Record<Category, CategoryGroup>` and `CATEGORY_LABELS`
carries `satisfies Record<Category, string>`, so a missing key fails the type-check.
The compiler enforces steps 5.1–5.3 once step 5.4 has run.

### 6. Reaching the stored rows

A dictionary change only touches postings written after it. To reach the existing
catalogue:

1. `cmd/backfill-derive` — `BACKFILL_CONCURRENCY` at 2–3. Six degraded prod.
2. `systemctl stop freehire-reindexw.timer` **before** any manual reindex, then
   `cmd/reindex`.

Expect the reindex to stop the search-drain timer on its own; a dead timer plus a
growing outbox is normal for the duration.

## Testing

- `classify_test.go` — assert each acronym family resolves `occupational_safety`,
  including `HSE Manager` (the precedence case that proves the block outranks the bare
  `manager` fall-through) and `Senior EHS Specialist`.
- Negative cases, from the exclusions above: `Engineer (she/her)` must not resolve this
  category; `Risk Analyst` must not; `Patient Safety Attendant` must not.
- `vocab_test.go` — the existing partition and craft-subset assertions cover the three
  list edits without new tests.
- `nontech_test.go` — assert `ConfirmedNonTech("Инженер по охране труда", false)` is
  false, i.e. the veto holds and the ingest filter no longer turns it away.
- A corpus probe over live titles, rather than the hand-written list, is what proves
  coverage — a test written from the same list that produced the dictionary cannot say
  anything about what the list is missing.

## The skills half

A category with no skills behind it is a filter that selects but cannot rank. Measured
on the same 6,254 live acronym postings: 2,535 carry any skill at all, and only 706 have
ever been enriched — because `is_tech` is false for this population, so the LLM enrich
gate skips them by design. Whatever skills they carry came from the deterministic
`skilltag` dictionary.

What that dictionary put on them, top of the list:

| tag                    | count |
| ---------------------- | ----- |
| regulatory-compliance  | 1,029 |
| iso-9001               |   398 |
| stakeholder-management |   271 |
| ai                     |   183 |
| analytics              |   165 |
| sharepoint             |   134 |
| powerbi                |   122 |
| six-sigma              |   119 |

Three of those are right. The rest is an IT-shaped dictionary picking up incidental
words — and two entries are outright false positives: `express` (22) and `fiber` (21)
are the JavaScript framework and the Go framework, matched inside safety prose.

A grep for the HSE vocabulary in `internal/dict/skilltag/dictionaries.go` returns
nothing: no `osha`, no `nebosh`, no `iso-45001`, no `hazop`, no `loto`. The profession
has zero representation.

### How the list below was obtained

Not by writing down what an HSE job sounds like. 2,500 live descriptions from this
exact population were pulled off prod, HTML-stripped, and mined for 1–3-grams by
document frequency, with the existing `skilltag` vocabulary subtracted. A second,
targeted pass then measured a named credential list against the same corpus. The
percentages below are document frequency over those 2,500 postings — the mining pass
is what found the shape, the probe is what put numbers on it.

### Admit — core (≥10% of postings)

`osha` (27%), `iso-14001` (20%), `emergency-response` (20%), `iso-45001` (19%),
`ppe` / `personal-protective-equipment` (14%), `root-cause-analysis` (14%),
`environmental-compliance` (14%), `risk-assessment` (13%), `incident-investigation`
(13%), `safety-management-system` (13%), `industrial-hygiene` (12%), `epa` (11%),
`nebosh` (11%), `corrective-action` (29% as "corrective actions").

### Admit — secondary (2–9%)

`waste-management` (9%), `first-aid` (9%), `hazardous-waste` (8%), `toolbox-talks` (8%),
`safety-audit` (8%), `hazard-identification` (7%), `contractor-safety` (6%),
`lockout-tagout` / `loto` (6%), `iosh` (5%), `confined-space` (5%), `nfpa` (5%),
`cpr` (5%), `stormwater` (4%), `ergonomics` (4%), `fall-protection` (3%),
`machine-guarding` (3%), `hot-work` (3%), `near-miss-reporting` (3%),
`process-safety-management` (2%), `hazop` (2%), `permit-to-work` (2%),
`job-safety-analysis` (2%), `rcra` (2%), `hazwoper` (1%), `behavior-based-safety` (1%),
`working-at-height` (1%).

EHS platforms, the tool half of the craft: `enablon` (1%), `intelex` (1%), plus
`velocityehs`, `sphera` and `cority` — each below the mining threshold here but they are
the named products in this market and cost nothing to carry.

### The acronym collisions, and the one that needs a decision

Several of these are three letters that already mean something else in this catalogue.
`skilltag` has a mechanism for exactly this — `categoryScopedAcronyms`, which resolves an
acronym only when the caller supplies a category on its allow-list — and the new
category is what makes it usable here.

- `CSP` — Certified Safety Professional, but also Content Security Policy and Cloud
  Service Provider. Category-scoped to `occupational_safety`.
- `CIH` (Certified Industrial Hygienist), `CHMM` (Certified Hazardous Materials
  Manager), `BBS` (Behavior-Based Safety, vs Bulletin Board System) — same treatment.
- **`PSM` is the one that does not fit.** The key is already taken:
  `dictionaries.go:1639` maps it to `professional-scrum-master`, scoped to
  `project_management`. `categoryScopedAcronyms` is a `map[string]categoryScopedAcronym`
  — one key, one canonical — so a second meaning cannot be added without widening the
  struct to hold a canonical per category.

  **Recommendation: do not widen it.** Admit only the spelled-out
  `process safety management`. The measured cost is nil: the acronym and the phrase each
  appear in 2% of postings, so the phrase alone loses almost nothing, and the structural
  change would touch every existing entry for one term.

- **Reject `ASP` and `DOT` outright.** Associate Safety Professional collides with
  ASP.NET; Department of Transportation collides with the English word. Neither survives
  a corpus check — the 17% this probe reported for `ASP` is the prefix of "aspects" and
  "aspiring", which is itself the reminder that a bare three-letter alias must be
  measured before it is admitted.

### Where the credentials belong

`internal/dict/certification` is a separate vocabulary holding PMP, CISSP, CKA and the
cloud certificates. NEBOSH, IOSH, CSP, CIH, CHMM and HAZWOPER are certifications of the
same kind and belong there rather than in `skilltag`. The implementation plan should
split them across the two dictionaries rather than pile everything into one.

## Expected effect

~6,200 acronym postings and several thousand `safety` postings gain a category. 4,515
stop being invisible to every facet. 1,454 leave `management`. The subscriber can tick
one box and see their profession.

On the skills side: ~3,700 postings that carry no skill at all gain one, and the ones
that already carry `express` or `fiber` stop lying. Because this population is
`is_tech = false` and never reaches the LLM, the dictionary is the only thing that can
put skills on it — there is no enrichment pass waiting to fix this later.

## Out of scope

- Financial and credit risk roles. They are a different profession that happens to
  share a word, and this change leaves them exactly as it found them — which for a bare
  "Risk Analyst" means no category at all, not the `finance`/`legal` an earlier draft of
  this line claimed.
- Industry tagging (oil & gas as a sector) — a separate facet with its own dictionary.
- Widening `categoryScopedAcronyms` to hold a canonical per category. See the `PSM`
  decision above; revisit only if a second collision of that shape appears.
