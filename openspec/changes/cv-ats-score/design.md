## Context

The verdict page already computes market-coverage from the profile's skills
against a role's live Meili facets (`internal/verdict`, `resume_verdict.go`). CVs
are stored per-user (S3 pointer on `users`, migration 0039) and parsed to skills
via `skilltag.Parse` (now separator/acronym-fixed). The old LLM "coherence" and
its server LLM client were removed. This change adds a CV ATS-readiness score —
deterministic structural + keyword checks plus an optional nil-safe LLM layer —
as a second section on the verdict page.

## Goals / Non-Goals

**Goals:**
- A concrete ATS score (0-100) + a fix checklist, computed from the CV text.
- Deterministic spine (reproducible, free); LLM layer strictly optional/nil-safe.
- Reuse the verdict's facet machinery and the fixed `skilltag` matcher.
- Keep `internal/atscheck` pure and unit-tested, like `internal/verdict`.

**Non-Goals:**
- Layout-aware PDF parsing (multi-column/table hard-detection) — the extractor is
  plain-text; we detect what text allows and let the LLM flag garbled text.
- Re-scoring market-coverage (unchanged).
- Per-job tailoring beyond the selected role.

## Decisions

### D1 — `internal/atscheck` pure scorer

`Score(cv CVText, role RoleSkills) Report` — no I/O. `Report{ Overall int;
Readability int; KeywordMatch int; Checks []Check }`, `Check{ ID string; Status
(pass|warn|fail); Label string; Fix string }`. Deterministic checks:

| id | rule | status |
|----|------|--------|
| `machine_readable` | extracted text length ≥ threshold | fail if near-empty (scan) |
| `contact` | email regex AND phone regex present | warn if one missing, fail if both |
| `sections` | curated heading dict (experience/education/skills, EN+RU) | warn per missing core section |
| `dates` | year or mm-yyyy pattern present | warn if none |
| `length` | word count in band (e.g. 150–1000) | warn if out of band |
| `bullets` | bullet markers present | warn if none |
| `keyword_match` | role top-N skills present as literal `skilltag` matches | scored (see D2) |

`Readability` = weighted pass-rate of the structural checks; `KeywordMatch` = D2;
`Overall` = `round(wR·Readability + wK·KeywordMatch)` (+ `wQ·contentQuality` when
the LLM ran). Weights are named constants (calibrate later, like
`verdict.MustHaveShare`).

### D2 — Keyword-match, distinct from market-coverage

`roleTopSkills` = the top-N of the role's `skills` facet (same `FacetCounts` the
verdict uses). `cvSkills` = `skilltag.Parse(cvText, WithResumeAcronyms())`.
`matched = cvSkills ∩ roleTopSkills`; `KeywordMatch = round(len(matched)/N·100)`.
The `keyword_match` check's `fix` names the top missing role skills.

*Why distinct:* market-coverage measures the profile's skill SET vs the whole
market (breadth); keyword-match measures whether the CV TEXT literally surfaces
this role's terms (what a keyword-filtering ATS scans). A CV can cover the role
yet fail the keyword scan if the terms aren't written out.

### D3 — Optional LLM layer, nil-safe (re-added server client)

`atscheck.Analyzer` (mirrors the deleted `coherence.go`): `Analyze(ctx, cvText)
→ *Review` over `llm.Client` (nil ⇒ `(nil,nil)`). `Review{ ContentQuality int;
Findings []Finding }` — weak/passive verbs, achievement-vs-responsibility,
garbled-text flag, 2-3 fixes. `GenerateJSON` + Sanitize/Validate (clamp, bound
strings). Re-add `config.Config` LLM/Langfuse fields + `cmd/server`
`llm.NewClient` construction (nil-safe; gated on `LLM_*`), passed to the handler.
No LLM ⇒ deterministic-only report (200), no `content_quality`.

### D4 — Cache the LLM review per-user, on the CV pointer

The LLM review is CV-only (role-independent) and metered/non-deterministic, so
cache it once per stored CV: add `users.resume_ats_analysis JSONB` next to the
existing `resume_object_key`. `GET` merges it in; `POST` computes + writes it.
**Invalidate on CV change:** `resume.Put`/`Delete` clear the column so a new CV
isn't scored with a stale review. Raw CV text is never stored — only the derived
review (same invariant as the old coherence blob).

*Alternative considered:* per-profile cache (a `search_profiles` column, like the
dropped `resume_analysis`). Rejected: the review is role-independent, so a
per-profile cache re-runs the LLM and yields inconsistent scores for one CV across
profiles. Per-user keyed to the CV is correct and cheaper.

### D5 — Endpoint, reuses verdict machinery

`GET /me/profiles/:id/ats-report` (owner-scoped 404; 503 when `facets == nil`):
1. resolve profile; read stored CV text (`resume.Text(userID)`).
   - No CV stored (storage on, none present) ⇒ 200 with `{ has_cv: false }` so the
     SPA shows the upload control (not an error).
2. build the role filter from the request facet params (same `roleValues` as
   verdict) → `FacetCounts(skills)` → `roleTopSkills`.
3. `atscheck.Score(cvText, roleTopSkills)`, merge cached review, return.
`POST` runs `Analyzer.Analyze` over the stored CV, caches, returns merged (best-
effort: LLM error ⇒ 200 deterministic, like the old verdict POST). CV upload is
the existing `ExtractResumeSkills`/résumé-storage path; the page gets an
upload/replace control.

### D6 — Delivery phased in one change

Tasks are ordered so **Phase 1 (deterministic)** is complete and shippable with
no LLM and no migration: `internal/atscheck` core, `GET` endpoint, verdict-page
section + CV upload, contracts. **Phase 2 (LLM)** adds the analyzer, the re-added
server LLM client, the `users` cache migration, the `POST` endpoint, and the
"Run AI review" UI. If we ship Phase 1 alone, the score is deterministic-only —
which is exactly the nil-LLM degradation, so no throwaway work.

## Risks / Trade-offs

- **Plain-text extraction is imperfect** (spacing, multi-column) → readability
  checks are heuristic; `machine_readable` catches the worst case (scans), and the
  LLM garbled-text flag covers soft multi-column. Copy states the limitation
  honestly; we never claim a full ATS simulation.
- **Weights uncalibrated** → named constants, documented as tunable; ship
  reasonable defaults and revisit with real CVs.
- **Re-adding the server LLM client** re-introduces config the verdict refactor
  removed → keep it nil-safe and isolated; enrich workers keep their own
  `config.Enrich` (unchanged).
- **Cache staleness on CV replace** → invalidate in `resume.Put`/`Delete`; a
  missed invalidation only serves an old review, never wrong CV text.

## Migration Plan

1. Phase 1 ships with no schema change.
2. Phase 2: migration adds `users.resume_ats_analysis JSONB` (nullable);
   manual-apply on prod before the Phase-2 binary (dictionary/`SELECT *` caveat —
   the users query is `SELECT *`, so apply the ADD COLUMN BEFORE rolling the
   binary, per the add-column convention). Regen sqlc. No reindex (no Meili attr).

## Open Questions

- Exact `roleTopSkills` N and the score weights — start with N=20 and equal-ish
  weights; calibrate against real CVs (the user's CV is a ready test input).
