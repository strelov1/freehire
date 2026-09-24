## Why

The catalogue cannot see the occupational-safety profession. Measured on prod on
2026-09-24, the acronym spellings alone (HSE, EHS, HSSE, QHSE, HSEQ, SHEQ, SHES) match
6,254 live postings: 4,515 carry no `category` at all — unreachable from every facet —
and 1,454 resolve `management`, not because anything understood them but because the
bare `manager` fall-through at `dictionaries.go:1047` sits above any block that would
outrank it. An HSE Manager files next to a Sales Manager. Widening to the word `safety`
adds several thousand more uncategorised postings. The whole dictionary holds exactly
one relevant line today: `{"safety engineer", "industrial_engineering"}`.

Both national occupational classifications give the profession its own unit and neither
files it under engineering — SOC minor group `19-5000 Occupational Health and Safety
Specialists and Technicians`, and ISCO-08 unit group `2263 Environmental and
occupational health and hygiene professionals`. Folding HSE into
`industrial_engineering` would make the facet lie: an HSE Manager is a compliance and
controls role, not a plant engineer.

## What Changes

- Add `occupational_safety` to `vocab.CategoryValues`, `vocab.NonTechCategories` and
  `vocab.NonTechCraftCategories`. Craft membership is load-bearing: `cmd/prune`'s
  business rule SUBTRACTS that set, and without it the retirement of one oil & gas or
  construction board would delete that employer's whole catalogue.
- Add an HSE alias block to the category title dictionary, placed **above** the bare
  `{"manager", "management"}` fall-through so a functional HSE prefix wins. Covers the
  acronyms, the spelled-out forms (`health and safety`, `environmental health and
  safety`, `occupational health and safety`, `охрана труда`), and the SOC reported
  titles (`safety officer/specialist/coordinator/advisor/supervisor/technician`,
  `industrial hygienist`, `risk control consultant`). `safety engineer` moves here from
  `industrial_engineering`.
- Keep bare `she`, bare `risk` and bare `safety` OUT. `she` is an English word and a
  pronoun that appears in live titles (`Engineer (she/her)`); `risk` matches 41,468
  live postings that are mostly finance; bare `safety` names other professions
  (`Patient Safety Attendant`, `Public Safety Officer`, `Food Safety`). Qualified forms
  only.
- Make `classify.ConfirmedNonTech` veto on `occupational_safety` the way it already
  vetoes on `engineering_design`. `nontechTerms` already carries `"охрана труда"` /
  `"охране труда"`, and `ConfirmedNonTech` feeds two DELETE paths (the ingest catalogue
  filter and the prune title rule) — without the veto the new category is cosmetic and
  the postings never survive ingest.
- Give the profession a skill vocabulary. It has none today: no `osha`, no `nebosh`, no
  `iso-45001`, no `hazop`. Of the 6,254 live postings only 2,535 carry any skill and
  only 706 were ever enriched (`is_tech` is false, so the LLM gate skips them by
  design) — the deterministic dictionary is the only thing that can tag this
  population.
- Scope the ambiguous HSE credential acronyms (`CSP`, `CIH`, `CHMM`, `BBS`) through the
  existing `categoryScopedAcronyms` mechanism, which the new category makes usable.
  Admit `process safety management` spelled-out only — the `PSM` key is already taken by
  `professional-scrum-master` and the map holds one canonical per key. Reject bare `ASP`
  and `DOT` outright.
- Serving surfaces: `CATEGORY_LABELS` → `Health & Safety (HSE)` in web and extension,
  `CATEGORY_GROUP` → `Quality & Security` (the group already holds `qa` and `security`,
  and QHSE/HSEQ bundle Quality with HSE themselves), plus regenerated contracts.

Not breaking: no column, index or API shape changes. A resolved `category` already flows
through `is_tech` derivation and the search index with no further code change.

## Capabilities

### New Capabilities
- `occupational-safety-category`: the category exists as a facet value with a defined
  membership — which title families resolve to it, which lookalike words must NOT, that
  its block outranks the bare `manager` fall-through, and that a resolved posting
  survives both catalogue delete paths.
- `occupational-safety-skills`: the deterministic skill vocabulary for the profession —
  the admitted terms, the certification/skill split, and the acronym-collision rules
  that keep three-letter HSE credentials from claiming IT postings.

### Modified Capabilities
- `design-taxonomy`: its "A resolved engineering-design category vetoes deletion"
  requirement closes with "Non-technical categories other than `engineering_design` MUST
  keep their current behaviour" — a sentence a second vetoing category contradicts. The
  requirement is restated so the veto belongs to the resolved *craft* categories, naming
  both members.

## Impact

- Code: `internal/dict/vocab/vocab.go` (three lists), `internal/dict/classify/
  dictionaries.go` (alias block above line 1047; `safety engineer` moved),
  `internal/dict/classify/nontech.go` (`ConfirmedNonTech` veto),
  `internal/dict/skilltag/dictionaries.go` (skill terms + category-scoped acronyms),
  `internal/dict/certification/certification.go` (NEBOSH, IOSH, CSP, CIH, CHMM,
  HAZWOPER), `web/src/lib/labels.ts`, `web/src/lib/filterSections.ts`,
  `extension/lib/labels.ts`, and `web/src/lib/generated/contracts.ts` via
  `cmd/gen-contracts`.
- Type-checked surfaces: `CATEGORY_GROUP` is `Record<Category, CategoryGroup>` and
  `CATEGORY_LABELS` carries `satisfies Record<Category, string>`, so a missing key fails
  the web build once the contracts are regenerated.
- `catalog-pruning`'s business rule is unchanged in requirement — the new category
  simply joins the set that rule already subtracts.
- Operational, out of scope for this change: existing rows keep a stale empty
  `category` until `cmd/backfill-derive` (`BACKFILL_CONCURRENCY` 2–3; six degraded
  prod) and a reindex are run, with `freehire-reindexw.timer` stopped first. Same
  pattern as prior dictionary-coverage backfills.
- Out of scope: financial and credit risk roles (a different profession sharing a word,
  left exactly as found — a bare "Risk Analyst" resolves no category, before and after);
  oil & gas as an industry tag (a separate facet);
  widening `categoryScopedAcronyms` to hold a canonical per category.
- Also out of scope, and recorded because it was checked: the `express` (22) and `fiber`
  (21) tags on this population are not the false positives they look like. Both aliases
  are already `ambiguousWords`-gated, and the `fiber` postings are fibre-optic
  manufacturers (OFS, AFL) hiring EHS staff — the tag describes the employer. Whether
  the gate leaks on `express` is a separate investigation with its own evidence.
