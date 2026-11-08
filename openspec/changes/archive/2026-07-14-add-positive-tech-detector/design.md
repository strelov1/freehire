## Context

`is_tech` (shipped in add-is-tech-facet) is a tri-state derived facet: `true` only when the title resolves a recognized technical **category**, `false` via the non-tech category blacklist or `classify.IsNonTech`, else `nil`. The asymmetry (two `false` sources, one narrow `true` source) undercounts tech: prod shows 294K tech vs 1.84M unknown, of which a narrow title-regex flags ≥170K as obviously-tech ("Software Engineer", "Web3 Developer"). A prod sample of unknown+senior titles confirms the mix — real software roles (Software Engineer, Salesforce/Web3 Developer, IT Administrator) sit beside non-software engineering (Mechanical/Manufacturing/Drainage/Project Engineer, Geologist).

## Goals / Non-Goals

**Goals:**

- Reclaim confidently-technical titles from `unknown` into `tech`, symmetric to `IsNonTech`.
- Keep the "never guess" / never-false-positive bias: a non-software "…Engineer" stays `unknown`, never wrongly `tech`.

**Non-Goals:**

- No schema/wire/facet change — `is_tech` column, jobview field, Meili facet, and the UI filter are untouched.
- No change to the `category` vocabulary — the detector feeds `is_tech` directly, not category.
- No description-based tech detection (title-only, like `IsNonTech`).

## Decisions

**1. A positive tech detector, not a wider category dictionary.**
Add `classify.IsTech(title) bool` backed by a new `techTitleTerms` list (whole-word via `wordmatch.UnicodeBoundary`). Rationale: mirrors `IsNonTech` exactly, keeps `category` semantics clean (a generic "Software Engineer" that names no sub-discipline shouldn't be forced into `backend`), and isolates the tech/non-tech question from the which-role question.

**2. The "engineer" trap governs the vocabulary.**
The prod sample proves bare "engineer" is dominated by non-software roles (mechanical, manufacturing, civil, drainage, optical, project) — so the dictionary uses **software/IT-anchored** terms only: "software engineer/developer", "web/backend/frontend/fullstack/mobile developer", "programmer", "devops", "sre"/"site reliability", "data scientist", "machine learning engineer", "system administrator"/"sysadmin", "cloud engineer", "security engineer", "qa engineer", "database administrator"/"dba", "<language> developer" (python/java/golang/…), "salesforce/web3/ios/android developer", etc. Bare "engineer"/"analyst"/"architect"/"consultant" are excluded. A test locks representative traps (Mechanical/Sales/Project Engineer, Geologist) to not-tech.

**3. Symmetric precedence in deriveIsTech.**
`true` if `slices.Contains(TechCategories, category)` OR `IsTech(title)`; else `false` if `NonTechCategories` contains category OR `IsNonTech(title)`; else `nil`. Tech is checked first, so a title with both a tech and non-tech term (rare) resolves tech — consistent with the existing "technical evidence wins".

## Risks / Trade-offs

- **False positive: a non-software role marked tech** → Mitigated by software-anchored terms only (no bare "engineer") + a trap-negative test set drawn from the real prod sample. Bias stays toward leaving ambiguous titles `unknown`.
- **Overlap with a non-tech term in one title** → tech-first precedence is deliberate; the sample shows near-zero genuine both-signal titles, and "software … " forms don't collide with the non-tech nouns.
- **Backfill/reindex cost at ~4.7M rows again** → same profile as the original ship; run off-peak, reindex after backfill (backfill's id-cursor stays ahead → the reindex captures final values).

## Migration Plan

1. Deploy the new binary (ingest now sets `is_tech=true` for tech titles).
2. `cmd/backfill-derive` to re-derive existing rows, then reindex so the reclaimed `unknown→tech` reaches the live facet.
3. Measure the new split; expect `tech` to roughly double and `unknown` to shrink.

Rollback: additive and idempotent — reverting the binary stops setting the new `true`s; a later backfill would re-`nil` them. No data migration.

## Open Questions

- None blocking. Remaining `unknown` after this (non-English titles, genuinely ambiguous) is acceptable and measurable.
