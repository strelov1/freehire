## Why

The `/my/profile/verdict` page under-sells its strongest asset: it scores a CV
against **live market demand**, but presents that as a single coverage percentage
and a flat pass/warn/fail checklist. Competing CV tools show a richer, more
actionable breakdown (per-skill status, category scores with point attribution,
concrete next steps). We can match that depth while keeping our differentiator —
every number is grounded in real vacancy data and the CV text, not invented by an
LLM — so the report is both more useful and still honest and testable.

## What Changes

- **Market coverage tab:** add a top-20 role-skill breakdown. Each skill is tagged
  `strong` / `hidden` / `missing`, flagged `must_have` when it appears in ≥ a
  threshold share of the role's open vacancies, and carries its market frequency
  and a status-specific advice line. Add three market-anchored headline stats:
  must-have covered (X/Y), stack-match %, and coherence % (declared skills backed
  by experience — an anti-buzzword-stuffing signal). The existing vacancy-coverage
  headline and gap list stay.
- **New deterministic capability:** section-aware CV parsing (split the CV text
  into a Skills section vs the body, tag skills in each) — the basis for
  `strong`/`hidden` status and the coherence score. Pure, I/O-free, no LLM.
- **CV readiness tab — BREAKING wire change:** restructure the ATS report from a
  flat checklist into five weighted categories (Keyword Strength 40 / Format 20 /
  Sections 15 / Content 15 / Length 10) with per-item point attribution, an
  `overall` = sum of category scores, a `potential` score (achievable if all fixes
  applied), a strong-keyword list, recommended-keyword chips, and a numbered
  suggestions list.
- **LLM role stays bounded:** the optional review still only supplies
  `content_quality` and `suggestions` (renamed from `findings`); it never produces
  a skill number. The whole feature works with no LLM configured.
- **Frontend:** rewrite `VerdictView.svelte` and `ATSReportView.svelte` against the
  new contracts using existing design-system tokens (status colours
  green/amber/red, no bespoke theme); regenerate the TS contracts.
- **Profile-page merge (IA):** fold `/my/profile/verdict` into `/my/profile` as one
  page on the `/jobs` layout (left filter sidebar + main column). Bring profile
  editing back inline as a `ProfileForm` (CV drop-zone with a "already uploaded ·
  update" state, skills + specializations selectors, one Save) — replacing the
  edit modal. The filter scopes the market comparison by role/facets, seeded from
  the profile's specializations but independent of it. Delete the `verdict` route
  and `ProfileEditModal`; drop the edit + sparkles header buttons (keep delete).
- **Deferred (out of scope):** a `(stack)/(methodology)` per-skill tag; a purple
  page theme.

## Capabilities

### New Capabilities
- `cv-section-parsing`: deterministic segmentation of CV plain text into a Skills
  section vs the body, with per-segment skill-tagging, producing `declared` /
  `body` / `all` skill sets.

### Modified Capabilities
- `resume-verdict`: add a market-anchored top-20 skill breakdown (per-skill
  status, must-have flag, advice) plus must-have-covered, stack-match, and
  coherence headline stats to the verdict; surface the whole feature on a single
  `/my/profile` page with inline editing and a role/facet comparison filter,
  removing the separate `/my/profile/verdict` route.
- `cv-ats-score`: replace the flat structural checklist score with five weighted
  categories carrying per-item point attribution, an additive `overall`, a
  `potential` score, and explicit strong/recommended keyword lists; the LLM
  content-quality always contributes to `overall` and `findings` is renamed
  `suggestions`.

## Impact

- **Backend:** new `internal/cvsection` (or `internal/resume` helper); `internal/verdict`
  (`Verdict` shape + `Compute`); `internal/atscheck` (`Report` shape + `Score` +
  `ApplyReview` + analyzer prompt); handlers `resume_verdict.go` / `ats_report.go`
  (extra role-skills facet query, CV-section wiring).
- **Contracts:** `cmd/gen-contracts` → `web/src/lib/generated/contracts.ts`
  (breaking shape change).
- **Frontend:** `VerdictView.svelte`, `ATSReportView.svelte`; new
  `ProfileForm.svelte`; rewritten `my/profile/+page.svelte`; deleted
  `my/profile/verdict/+page.svelte` and `ProfileEditModal.svelte`.
- **No DB migration:** reuses the stored CV text and the existing
  `users.resume_ats_analysis` cache column.
