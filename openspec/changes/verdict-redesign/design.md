## Context

`/my/profile/verdict` has two tabs: **Market coverage** (`internal/verdict` →
`VerdictView.svelte`) and **CV readiness** (`internal/atscheck` →
`ATSReportView.svelte`). Today the market tab shows one coverage % + a gap list,
and the CV tab shows a flat pass/warn/fail checklist with an optional LLM
content-quality re-blend. Both under-present the underlying data.

Constraints from the codebase:
- `internal/verdict.Compute` and `internal/atscheck.Score` are **pure, I/O-free**;
  handlers own the Meilisearch facet queries and the stored-CV read.
- The LLM layer is **optional** — the feature must fully work without it
  (mirrors `atscheck`/`enrich` conventions).
- Skill numbers must be grounded in **real market data** (Meili facets) and the CV
  text; the LLM writes prose only.
- No DB migration is wanted: reuse the stored CV text and the existing
  `users.resume_ats_analysis` cache column.

The full brainstormed design lives at
`docs/superpowers/specs/2026-07-03-verdict-redesign-design.md`.

## Goals / Non-Goals

**Goals:**
- Market tab: top-20 role-skill breakdown with `strong`/`hidden`/`missing` status,
  `must_have`, market frequency, advice; headline stats (must-have covered,
  stack-match %, coherence %) — all from real data.
- CV tab: five weighted categories with per-item point attribution, additive
  `overall`, `potential` score, strong/recommended keyword lists, numbered
  suggestions.
- Keep the feature fully functional with no LLM configured.

**Non-Goals:**
- `(stack)/(methodology)` per-skill tag (deferred).
- A bespoke purple page theme (deferred; use design-system tokens).
- Any DB schema change.

## Decisions

**1. Section-aware CV parsing as a new pure package.**
Add `internal/cvsection` (I/O-free, mirrors `atscheck`) that splits CV text into a
Skills section vs the body by heading detection, then runs
`skilltag.Parse(..., WithResumeAcronyms())` over each, yielding `declared` / `body`
/ `all`. *Why:* `strong`/`hidden` status and coherence both need Skills-section-vs-
body attribution, which whole-document `skilltag` can't give. *Alternative
considered:* have the LLM classify status — rejected (violates "numbers from real
data", not reproducible, costs tokens).

**2. Must-have by market-frequency threshold.**
A top-20 skill is `must_have` when it appears in ≥ `MustHavePct` (tunable const,
start 50%) of the role's open vacancies. *Why:* real-data-grounded and self-adjusts
per role. *Alternative:* fixed top-N — rejected (less honest across roles).

**3. Additive category model for the ATS score (max = weight).**
Five categories with fixed maxima summing to 100; `overall` = Σ category scores;
`potential` = `overall` + recoverable points. *Why:* transparent attribution and a
motivating target, simpler than the current weighted re-blend. Content Quality
always contributes — via the LLM score when present, else a deterministic proxy
(action-verb + quantified-number detection).

**4. LLM stays a bounded enrichment.** The `Review` supplies only `content_quality`
+ `suggestions` (renamed from `findings`); it never emits a skill number or a
category score. Cached per user in the existing column, invalidated on CV replace.

**5. One extra facet query, no new dependency.** The role facet query in
`computeCoverage` gains `Facets:["skills"]` to read the full role skill
distribution (frequency → must-have + top-20 ordering). The ATS report already
runs a `skills` facet query for its role top-N; both can share.

## Risks / Trade-offs

- **Heading detection on messy PDF-extracted text may miss the Skills section** →
  `declared` empty ⇒ everything reads as `hidden`, coherence 0. Mitigation:
  documented, safe fallback (never crash); the advice still makes sense ("add a
  Skills section"). Calibrate heading patterns against real CVs.
- **`cv-ats-score` is an unarchived OpenSpec change**, so its base spec isn't in
  `openspec/specs/` yet → the MODIFIED delta here can't resolve until `cv-ats-score`
  is archived. Mitigation: archive `cv-ats-score` before archiving this change, or
  archive them together; validated with `openspec validate` before implementation.
- **Breaking wire-shape change** to `Verdict` and `Report` → the SPA breaks until
  the frontend rewrite + regenerated contracts land. Mitigation: land backend,
  contracts, and frontend in the same change; no external API consumers.
- **Two coverage numbers on the market tab** (vacancy-coverage % and stack-match %)
  risk confusing users. Mitigation: distinct labels/hierarchy; they measure
  different things (weighted reach vs top-20 breadth).

## Migration Plan

No DB migration. Deploy backend + regenerated contracts + frontend together (single
change). Rollback = revert the change; stored CV text and the cache column are
untouched. Archive `cv-ats-score` first (or together) so the MODIFIED delta
resolves at sync time.

## Open Questions

- Final `MustHavePct` value (start 50%, calibrate post-launch).
- Package name: `internal/cvsection` vs a helper under `internal/resume` — decide at
  implementation; keep it pure regardless.
