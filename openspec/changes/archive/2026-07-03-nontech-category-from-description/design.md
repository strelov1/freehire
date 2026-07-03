## Context

`internal/classify` derives `seniority` and `category` deterministically from a job
**title** (`classify.Parse`), and additionally derives `seniority` from the
**description** via `SeniorityFromDescription` (intent-anchored phrases). `category`
has no description tier — the codebase comment in `jobderive.Derive` states
"Description prose is too noisy to derive a category deterministically." On prod this
leaves ~71% of open jobs with an empty `category`.

Step 1 added an enqueue gate that skips AI enrichment for jobs whose `category` is in
`enrich.NonTechCategories` (`marketing`/`sales`/`support`/`management`). Because the
empty-category majority falls through the gate, non-technical jobs that a title alone
did not classify are still enriched. This change adds a precision-anchored non-tech
category detector over the description, reusing the exact pattern already proven by
`SeniorityFromDescription`.

## Goals / Non-Goals

**Goals:**
- Recover a `category` for confidently **non-technical** roles from the description so
  they hit the existing gate and stop consuming LLM budget.
- Zero false positives that would mislabel a technical job as non-tech.
- Reuse the established derivation seam (`jobderive.Derive`) so both live ingest and
  `cmd/backfill-derive` are covered by one change.

**Non-Goals:**
- Detecting **technical** categories from the description (a title-empty tech job is
  already enriched as empty — classifying it tech saves no LLM cost). Out of scope.
- Russian-language phrase support (the existing `SeniorityFromDescription` is EN-only;
  RU is a noted future seam, not this change).
- Any change to the enqueue gate, schema, or the served facet doctrine.

## Decisions

**1. A dedicated non-tech-only detector, not a general category-from-description parser.**
The cost goal is served entirely by non-tech recall; a full 25-value description
classifier is more surface, more risk, and no extra savings. `NonTechFromDescription`
returns only `{marketing, sales, support, management}` or empty. (Alternative: extend
to all categories — rejected by scope; the function stays a small, auditable table.)

**2. Role-statement anchors only; never bare words.** A description mentioning "work
with our sales team" or "our support engineers" must not fire. Phrases anchor the role
to a hiring statement (`"sales representative"`, `"account executive"`, `"customer
success"`, `"office manager"`). Tech-adjacent titles are explicitly excluded from the
tables (`"sales engineer"`, `"solutions engineer"`, and `engineering`/`product`/
`project`/`data` manager forms). This mirrors `SeniorityFromDescription`'s anchored-
phrase doctrine. (Alternative: also match section headers / repeated role terms —
rejected for higher false-positive risk.)

**3. Integrate as the third tier of the existing category precedence in
`jobderive.Derive`.** `category = structured → title dictionary → description non-tech`.
The detector only fills a `category` the higher sources left empty. This is the single
seam feeding both ingest and backfill. `cmd/tg-extract` derives via `classify.Parse`
directly (historical bypass of `jobderive`), so it gets the same fallback at its two
sites. (Alternative: a gate-only signal computed at enqueue — rejected; it duplicates
logic and would not populate the stored facet.)

## Risks / Trade-offs

- **False-positive non-tech → permanent enrichment loss.** Mislabeling a tech job as
  non-tech gates it forever (an empty category, by contrast, just keeps enriching).
  → Mitigation: precision-first phrase tables (role-statement anchors, tech-adjacent
  exclusions), explicit negative unit tests, and "emit nothing when unsure". The
  asymmetry deliberately favors under-detection.
- **`management` is the riskiest category in prose** ("engineering manager" is tech per
  the Step-1 decision). → Mitigation: `management` anchors are limited to
  unambiguously administrative roles; engineering/product/project/data manager forms
  are never matched.
- **Modest recall.** Anchored phrases will miss many non-tech jobs whose descriptions
  are phrased loosely. → Accepted: this is an additive, safe improvement, not a
  complete solution; the empty-category bucket remains the long-term limiter.

## Migration Plan

1. Ship the code (no migration). New ingests derive and gate automatically.
2. Run `cmd/backfill-derive` to re-derive `category` (incl. the new description tier)
   over existing jobs, then `make reindex` (facet columns changed) per the standing
   dict-change runbook.
3. Optionally delete already-queued non-tech rows from `enrichment_outbox`
   (`DELETE ... USING jobs WHERE category = ANY(NonTechCategories)`) to realize
   savings on the current backlog immediately.

Rollback: revert the code; `category` values already written by the detector are
harmless (they are valid vocabulary values) and are overwritten on the next re-derive.

## Open Questions

None — scope and precision policy were settled during brainstorming.
