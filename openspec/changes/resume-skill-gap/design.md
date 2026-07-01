## Context

Full approved design at `docs/superpowers/specs/2026-07-01-resume-skill-gap-profiles-design.md`.
This mirrors the key decisions for OpenSpec tracking.

The codebase already has the pieces: `internal/skilltag.Parse` (deterministic dictionary →
canonical skill slugs, same slugs as `jobs.skills` and the search facet), the `search_profiles`
table with `skills[]` / `specializations[]`, and the public `GET /api/v1/jobs/facets` endpoint
that returns per-skill open-job counts honoring a `category` filter. `SearchProfilesView.svelte`
already renders profile CRUD with a skills typeahead. Profiles and their endpoints are
cookie-only (`RequireAuth`).

## Goals / Non-Goals

**Goals:**
- Extract skills from an uploaded resume (PDF or pasted text) using the existing dictionary.
- Merge extracted skills into a profile without clobbering user-entered skills.
- Show, per profile, how many of the top-20 market skills for its specialization(s) are missing.
- Add exactly one new backend endpoint; compute the gap on the frontend.

**Non-Goals:**
- No LLM extraction and no Hirable-style "coherence score" (skills vs experience).
- No storage/history of resumes; no re-analysis from DB.
- No auto-detection of specialization from the resume — the user picks it.
- No separate `/resume` page; no DB schema change.

## Decisions

- **Deterministic extraction, not LLM.** `skilltag.Parse` is instant, free, and emits the exact
  slugs the market facet uses, so extracted skills and market data align 1:1 with no mapping.
  Trade-off: only ~250 dictionary skills are found. Alternative (LLM) rejected for MVP: cost,
  latency, and a canonicalization step.
- **Server-side PDF parsing; resume not stored.** The endpoint accepts a PDF (multipart) or text
  (JSON), extracts via `github.com/ledongthuc/pdf`, runs `skilltag.Parse`, and returns slugs.
  The file lives only in request memory. Keeps the dictionary in one place (Go) and avoids
  shipping/porting it to the browser. Alternative (client-side pdf.js) rejected: would duplicate
  extraction logic in JS.
- **Oversize protection = existing global body limit.** The server already sets fiber
  `BodyLimit` (currently 1 MB), which 413s an oversize body before the handler runs. We rely on
  that rather than adding a redundant per-handler size check — text and typical text-based PDF
  resumes fit comfortably; image-heavy multi-MB PDFs are out of scope (they extract poorly). If
  larger uploads are needed later, raising the limit is a separate, deliberate change.
- **Gap on the frontend over `/jobs/facets`.** For a profile's specializations the frontend
  calls `/jobs/facets?category=<spec>&category=<spec>` (OR), sorts the `skills` facet desc,
  takes top N=20; `missing = topN − profile.skills`. Same frontend-only pattern as
  filter-collections. The only new backend surface is extraction.
- **Multi-spec = OR-combined market.** The facets `category` filter is OR across values, so a
  profile's specializations naturally yield one combined top-20 — no manual aggregation, no
  single-role selection like Hirable.
- **Merge, don't overwrite.** Uploading unions extracted skills into the form's current skills
  (dedup); the user edits before saving via the existing create/update endpoints.
- **N=20 as a constant.** Mirrors Hirable's "top 20 stack"; a single named constant, easy to tune.

## Risks / Trade-offs

- [Dictionary misses rare/novel skills] → Acceptable for MVP; the seam to add an LLM pass later
  is noted, and the same dictionary powers market data so at least both sides are consistent.
- [PDF text extraction quality varies by resume layout] → `ledongthuc/pdf` handles standard
  text PDFs; scanned/image PDFs yield little text and simply produce fewer skills (graceful
  degradation, returns `[]` rather than erroring).
- [Resume PII passes through the server] → Mitigated by never persisting or logging the file/text
  and returning only canonical slugs.
- [`/jobs/facets` returns few skills for a thin category] → Coverage denominator becomes < 20;
  `computeGap` uses `min(N, available)` so the ratio stays honest.
- [New go.mod dependency] → `ledongthuc/pdf` is small and text-only; contained to `resume.go`.

## Migration Plan

No schema change, no data migration. Backend deploy adds the endpoint and dependency; frontend
deploy adds the UI. Rollback is a plain revert — nothing is persisted that would need cleanup.

## Open Questions

None. (Resolved in brainstorming: N=20; gap on frontend; missing chips link to `/jobs` as the
last, cuttable item.)
