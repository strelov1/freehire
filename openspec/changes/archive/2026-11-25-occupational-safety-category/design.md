## Context

`internal/dict/classify` resolves `jobs.category` by whole-word match against one
ordered table whose order encodes precedence. The vocabulary in `internal/dict/vocab`
splits every category three ways — technical, non-technical, and non-technical *craft* —
and those splits decide whether a posting is enriched, indexed, or deleted.

The occupational-safety profession has no seat in any of that. Measured on prod on
2026-09-24 over live, non-duplicate postings: the acronym spellings match 6,254; 4,515
resolve nothing and 1,454 resolve `management` off the bare `manager` fall-through. The
dictionary's only relevant line is `{"safety engineer", "industrial_engineering"}`.

Two constraints shape the design more than anything else:

1. **This population is `is_tech = false` and always will be.** The enrichment enqueue
   gate reads `is_tech IS TRUE`, so the LLM never sees these postings. Whatever
   category and skills they get, the deterministic dictionaries must supply. There is no
   later pass that fixes a gap here.
2. **`nontechTerms` already names this profession in Russian.** `"охрана труда"` and
   `"охране труда"` sit in that list, and `ConfirmedNonTech` feeds two DELETE paths —
   the ingest catalogue filter and the prune title rule. Adding a facet without adding a
   veto produces a category that postings cannot reach.

The design reference with the full production measurements and the classification
research is `docs/superpowers/specs/2026-09-24-occupational-safety-category-design.md`.

## Goals / Non-Goals

**Goals:**

- A subscriber who works in HSE can tick one facet and see their profession, whatever
  way the employer spelled it.
- The 6,254 acronym postings resolve a category that is true, rather than an empty
  string or `management`.
- The profession's postings survive ingest and prune rather than being deleted on the
  strength of a Russian non-tech term match.
- The postings carry skills that describe safety work. Today most carry none at all:
  2,535 of 6,254 have any skill, and the dictionary has no HSE vocabulary whatsoever.

**Non-Goals:**

- Financial and credit risk roles. A different profession that shares a word. This
  change leaves them exactly as it found them, which for a bare "Risk Analyst" means
  resolving NO category — a review caught an earlier draft claiming they "already
  resolve `finance` and `legal`", which is not what the dictionary does. Their coverage
  is a separate question from this one.
- Oil & gas as an industry. That is `industrytag`'s facet and its own change.
- Widening `categoryScopedAcronyms` to hold one canonical per category. See the `PSM`
  decision below.
- Backfilling and reindexing the existing rows. Operational, follows the merge.

## Decisions

### Why a new category rather than a fold into `industrial_engineering`

Both national occupational classifications give the profession its own unit, and
neither files it under engineering: SOC minor group `19-5000` (under *Life, Physical and
Social Science*, split into `19-5011` specialists and `19-5012` technicians) and ISCO-08
unit group `2263` (under *Other Health Professionals*, whose ESCO concept `2263.3` is
literally `health and safety officer`). SOC's own reported-title list for `19-5011`
names EHS Officer, Safety Specialist, Risk Control Consultant and Industrial Hygienist.

*Alternative considered:* fold into `industrial_engineering`, which already holds the
one `safety engineer` line. Rejected — the facet would lie. That category is defined in
`vocab.go` as the seat a factory, plant or utility staffs for manufacturing, process,
quality, maintenance and commissioning work. An HSE Manager is a compliance and controls
role; a subscriber selecting "Industrial Engineering" would get a factory mixed with a
safety department, and vice versa.

*Alternative considered:* follow LinkedIn, which has no HSE function at all and spreads
these people across Engineering, Operations and Quality Assurance. Rejected on
granularity — that taxonomy has 26 functions for the entire labour market and no
Customer Success function either. Ours runs ~45 categories, which is SOC-minor-group
resolution, so the government classifications are the applicable precedent.

### Key name `occupational_safety`, label `Health & Safety (HSE)`

The key follows both canonical group names. `health_safety` was rejected: the catalogue
already has `healthcare`, and the two would read as siblings.

The label carries the acronym on purpose. It is what the population calls itself, and it
makes the filter findable by the word the subscriber already types.

### All three vocabulary lists, including the craft list

`NonTechCraftCategories` membership is the non-obvious one and it is load-bearing.
`cmd/prune`'s business rule deletes non-technical categories at a company with no
technical history, and it SUBTRACTS that set. Without membership, the moment an oil &
gas or construction employer's board is retired, prune takes out that employer's entire
catalogue — precisely the failure the set was created to prevent for
`engineering_design` and `industrial_engineering`.

An existing test asserts the three sets partition `CategoryValues` exactly and that
every craft member is also a non-tech member; both must stay green.

### The alias block goes above the bare `manager` fall-through

`dictionaries.go:1047` is `{"manager", "management"}`, documented as the fall-through for
a manager title with no recognised function. It is what currently claims 1,454 HSE
postings. The table's order is its precedence, so the HSE block must precede it — the
same placement logic the file already applies to `community manager`, which sits above
the fall-through so the unqualified title still resolves.

This deliberately diverges from SOC, which codes EHS Managers under a management code
separate from `19-5011`. Our `category` facet names the craft and `seniority` is a
separate facet, so splitting one profession across two category values would force the
subscriber to tick two boxes to see one job market.

### What stays out of the dictionary, and why each was measured

- **Bare `she`.** An ordinary English word, and live titles carry pronouns
  (`Engineer (she/her)`). Qualified forms only: `she manager`, `she officer`,
  `she advisor`, `she coordinator`, `she specialist`. Same precedent the file already
  sets for bare `security` and bare `mobile`.
- **Bare `risk`.** 41,468 live postings match the word and most are finance. Only
  `hse risk` and `safety risk`, plus SOC's `risk control consultant`.
- **Bare `safety`.** Names other professions in live titles — `Patient Safety Attendant`
  (healthcare), `Public Safety Officer` and `Campus Safety Officer` (protective
  services), `Food Safety` (manufacturing QA). The qualified title forms cover the
  population without it.

### The skill vocabulary was mined, not written

A list written from what an HSE job sounds like can only be tested against itself. 2,500
live descriptions from this exact population were pulled off prod, HTML-stripped and
mined for 1–3-grams by document frequency with the existing `skilltag` vocabulary
subtracted; a second targeted pass then measured a named credential list against the
same corpus. Every term admitted carries its measured document frequency.

That second pass is also the argument for the method: it reported `ASP` at 17%, which
turned out to be the prefix of "aspects" and "aspiring". A bare three-letter alias that
is not measured against live text does not belong in the table.

### `PSM` keeps its existing meaning; the spelled-out phrase carries HSE

`categoryScopedAcronyms` is `map[string]categoryScopedAcronym` — one key, one canonical,
with a category allow-list. `PSM` is already `professional-scrum-master` scoped to
`project_management`, and Process Safety Management cannot join it without widening the
struct to hold a canonical per category.

*Decision:* do not widen it. Admit `process safety management` spelled out only. The
measured cost is nil — the acronym and the phrase each appear in 2% of the corpus — and
the structural change would touch every existing entry for one term. `CSP`, `CIH`,
`CHMM` and `BBS` have no such collision inside the mechanism and are scoped to
`occupational_safety` normally.

### Certifications go to the certification dictionary

`internal/dict/certification` already holds PMP, CISSP, CKA and the cloud certificates.
NEBOSH, IOSH, CSP, CIH, CHMM and HAZWOPER are credentials of the same kind and belong
there rather than in `skilltag`, which names skills.

### `Quality & Security` as the filter group

That group already holds `qa` and `security`. QHSE and HSEQ bundle Quality with HSE in
the profession's own naming, so the grouping matches what the population calls itself.
*Alternative considered:* `Engineering`, next to `industrial_engineering`, on the
existing "that is where a plant engineer looks first" reasoning. Rejected — an HSE
Officer is not an engineer, and the group would repeat the fold this design already
rejected.

A category with no `CATEGORY_GROUP` entry is generated into the contracts but
unreachable in the picker, so this entry is load-bearing rather than cosmetic.

## Risks / Trade-offs

- **A new alias claims postings from a category that was right** → Every alias is
  declared with its measured document frequency, the exclusions above keep the loose
  words out, and a regression test asserts the three named lookalikes (`Engineer
  (she/her)`, `Risk Analyst`, `Patient Safety Attendant`) do not resolve the new
  category.
- **The veto is forgotten and the postings are deleted at ingest** → `ConfirmedNonTech`
  is the single funnel both DELETE paths go through, by construction. A test asserting
  `ConfirmedNonTech("Инженер по охране труда", false)` is false guards it.
- **The hand-written alias list misses spellings the market uses** → A corpus probe over
  live titles, rather than a test written from the same list that produced the
  dictionary, is what measures coverage. The mined corpus is the input for that probe.
- **`safety engineer` moving out of `industrial_engineering` changes existing rows** →
  46 postings, all of them better placed under the new category. The move is the point,
  not a side effect.
- **The change looks done at merge but no user sees it** → The dictionary only touches
  postings written after it. The migration plan below is not optional.

## Migration Plan

1. Merge. New postings resolve the category from the next ingest tick.
2. `cmd/backfill-derive` with `BACKFILL_CONCURRENCY` at 2–3. Six degraded prod; this is
   a measured limit, not a guess.
3. `systemctl stop freehire-reindexw.timer` **before** any manual reindex, then
   `cmd/reindex`. The reindex stops the search-drain timer itself; a dead timer plus a
   growing outbox is expected for the duration.
4. Verify by facet count on the live site, not by unit test.

Rollback is a revert of the dictionary entries; the facet value would then resolve
nothing and the postings return to their prior state on the next backfill.

## Open Questions

None blocking. The one structural question — whether `categoryScopedAcronyms` should
hold a canonical per category — is answered "not for this change" and recorded above;
revisit only if a second collision of that shape appears.

## Coverage as measured, not as assumed

Run after implementation, against live prod titles rather than against the list that
produced the dictionary (`TestCategoryCorpusProbe`, added for this purpose).

**The profession's own spellings — 3,869 distinct titles, 6,252 live postings carrying
an HSE acronym:**

| outcome | postings | share |
| --- | ---: | ---: |
| `occupational_safety` | 6,026 | **96.4%** |
| another category | 217 | 3.5% |
| unresolved | 9 | 0.1% |

The 9 are CJK-fused titles the word-boundary matcher cannot reach (`EHS工程师`,
`EHSアシスタントマネージャー`, `HSEQリーダー`). A handful more sit in the 3.5% because an
exclusion qualifier legitimately fires first — "EHS Food Safety Specialist" is genuinely
HSE but loses to `food safety`. At that count it is not worth a precedence exception.

**The deliberately wide net — every live title carrying `hse|ehs|…|she|safety|hygiene|
охран`, 30,347 postings:** 46.4% resolve the category, 31.9% resolve nothing, 21.7%
resolve elsewhere. The "elsewhere" share grew and the "nothing" share shrank after
review: the qualifier exclusions now ROUTE rather than blank, so Patient Safety nurses
go to `healthcare` and Product Safety engineers to `industrial_engineering` instead of
resolving nothing. The unresolved share is mostly correct: that query was written to
over-collect, and it pulls in security guards (`младший инспектор отдела охраны`),
forest rangers (`государственный инспектор по охране леса`), campus and public safety
officers, deputy sheriffs and dental hygiene assistants — none of which is this
profession.

**What the probe found that the hand-written list had missed**, and which no test
written from that list could have surfaced: the Russian genitive `охраны труда` (112
postings in one title alone), the inverted `Director of Safety` (20), the Singapore
`Workplace Safety and Health` family (25 across spellings), and seven more
`Safety <role>` forms — administrator, professional, trainer, intern, representative,
inspector. Adding them moved the wide-net figure from 43.9% to 47.3%; the review's
qualifier exclusions then moved it back to 46.4% by handing ~300 postings to the
professions they actually belong to.

## What the review changed, and why it is recorded here

Two decisions in this document were wrong as written, and the corrections belong beside
them rather than only in a commit message.

**The qualifier exclusions must route, not blank.** The first implementation gave
`food safety`, `public safety`, `patient safety`, `campus safety` and `product safety`
the blind sentinel. That blanked titles which already resolved correctly and had nothing
to do with HSE — "Patient Safety Registered Nurse" lost `healthcare`, "Product Safety
Engineer" lost `industrial_engineering`, "Public Safety Dispatcher" lost `logistics`.
`dictionaries.go` states the rule that forbids this (the sentinel is only for phrases
with no better category) and cites the last time it was broken. Each qualifier now routes
to the category that is true; the sentinel survives only for protective services and
Trust & Safety, and is narrowed to the role noun so it stops taking other people's
answers.

**The exclusion list was measured on the wrong board.** It covered the five consumer
collisions and missed every tech-native one. On an IT job board the likeliest "safety"
collision is **Trust & Safety** — platform integrity at a consumer-tech employer — which
was being filed under Health & Safety and deriving `is_tech = false` with it. So were
Functional Safety (ISO 26262), Drug Safety (pharmacovigilance), AI Safety (alignment),
Life Safety (fire-alarm trades) and school and pool safety officers. All are now excluded
or routed, each with a test.

**And the skill vocabulary needed the other half of its test.** Every assertion was
"HSE text contains HSE term"; none asked what these phrases do to a posting that is not
about safety. They are strong corroborators by default, so one of them released every
gated `ambiguousWords` token in the same text — a warehouse posting came back carrying
`react` and `sketch`. All 35 phrase canonicals joined `nonCorroboratingPhrases`, which is
where this file's own doctrine already put `hipaa`, `nist` and `iso-9001`. The five word
aliases (`osha`, `epa`, `nfpa`, `rcra`, `hazop`) stay corroborating: a regulator's name
is not English prose.

## Operational follow-up, owed after merge

The dictionary only touches postings written after it. Existing rows keep their stale
`category` and `skills` until:

1. `cmd/backfill-derive`, `BACKFILL_CONCURRENCY` at 2–3. Six degraded prod; this is a
   measured limit, not a guess.
2. `systemctl stop freehire-reindexw.timer`, then `cmd/reindex`. The reindex stops the
   search-drain timer itself, so a dead timer plus a growing outbox is expected for the
   duration.
3. Verify by facet count on the live site — the `Health & Safety (HSE)` option should
   appear under Quality & Security with roughly 6,000 postings behind it — not by unit
   test.
