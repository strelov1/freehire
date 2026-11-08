## Context

`classify.IsNonTech` feeds the `is_tech=false` branch. Its ~70-term dictionary catches only a fraction of the non-tech flooding the catalogue from enterprise ATS boards, so 58% of open jobs sit `unknown`. A cluster census of the unknown bucket sizes the reclaim: healthcare ~80K, warehouse/logistics ~62K, retail ~45K, food ~43K, trades ~31K, office/finance ~25K, education ~24K, facilities ~12K.

## Goals / Non-Goals

**Goals:**

- Move the obvious non-tech out of `unknown` into `non_tech` so "exclude Non-tech" cleans the catalogue.
- Keep zero false positives: a tech role must never be mislabelled non-tech.

**Non-Goals:**

- No change to the derivation, `is_tech` semantics, schema, wire, facet, or UI.
- No attempt to classify genuinely ambiguous titles — they stay `unknown`.
- No catalogue exclusion of non-tech (a separate, deferred product decision).

## Decisions

**1. Grow the existing `nonTechTitleTerms`, same doctrine.**
Add unambiguous role nouns across the eight measured clusters, whole-word via `wordmatch`. The dictionary stays alias-free prose forms (full role nouns), consistent with the current list.

**2. The collision list is the guardrail.**
The prod sample shows the traps: "IT Technician"/"Field Service Technician", "Data Warehouse Engineer", "Security Engineer", "Systems Coordinator", "Payroll... " vs "Data Analyst". So the expansion **excludes** bare "technician", "engineer", "analyst", "coordinator", "specialist", "administrator", "officer", "agent", "server", "warehouse", "chef", "operator" — using anchored forms instead ("warehouse associate", "hvac technician", "pharmacy technician"). A trap-negative test set (IT Technician, Data Warehouse Engineer, Security Engineer) locks this.

**3. Abbreviation care.**
Short forms like "rn"/"cna"/"lpn"/"cdl" are included only as whole words (word-boundary matched), where collision with tech tokens is negligible in job titles.

## Risks / Trade-offs

- **A tech role wrongly marked non-tech** → Mitigated by the collision exclusion list + trap-negative tests; bias stays toward leaving ambiguous titles `unknown`. Tech-first precedence in `deriveIsTech` also protects any title that resolves a tech category or `IsTech`.
- **Backfill/reindex cost at ~4.7M again** → same profile; run the chained `backfill-derive && reindex` off-peak.

## Migration Plan

1. Deploy the new binary.
2. Chained `cmd/backfill-derive && reindex` (backfill's id-cursor stays ahead of the reindex → final values captured).
3. Measure: expect `non_tech` up ~300-500K, `unknown` down, `tech` flat.

Rollback: additive/idempotent — reverting stops setting the new `false`s; a later backfill re-`nil`s them. No data migration.

## Open Questions

- Whether to follow with catalogue exclusion of `non_tech` (option B) — deferred to the post-expansion numbers.
