## Why

The profile "verdict" today answers a question users don't quite ask: "how many
of the top-20 in-demand skills do you have?" — a ratio over a skill list, plus a
bolted-on LLM "coherence" score that reads as a disconnected second metric.
Users think in vacancies: *how many real openings do my skills reach, and which
missing skill unlocks the most new ones?* This change reshapes the verdict into a
market-coverage tool answered directly from the live index, and drops the AI
coherence feature that never earned its place on the page.

## What Changes

- **Coverage is measured in real vacancies, not a skill-list ratio.** For the
  selected role(s), report the count and percent of open vacancies that list at
  least one of the profile's skills (`covered / total`), computed from
  Meilisearch — not "N of the top-20 skills".
- **Each missing skill shows the NEW vacancies it unlocks.** A gap skill's value
  is the number of currently-uncovered vacancies that list it (vacancies with
  that skill but none of the user's), shown as `+N` and `+X%`, ranked biggest
  win first.
- **The verdict page gets an interactive, sidebar-style filter.** The user
  changes role/specialization, seniority, region, etc. live (like `/jobs`) and
  coverage recomputes — **without editing the profile**. Absent a category
  filter, the calculation defaults to the profile's specializations.
- **UI split:** the profile list shows the headline numbers (coverage count +
  percent); the verdict page shows the full breakdown (coverage + ranked gaps).
- **BREAKING — the AI "Résumé coherence" feature is removed entirely:** the
  verdict-page coherence upload UI, `internal/verdict/coherence.go` (LLM
  analyzer), the `POST /me/profiles/:id/verdict` endpoint, and the
  `search_profiles.resume_analysis` column (dropped via migration). The
  deterministic verdict becomes the whole verdict.
- **Unchanged:** the profile form's résumé→skills extraction
  (`POST /me/resume/extract`) and the résumé-storage subsystem it shares stay as
  they are — this change only severs their use by the (removed) coherence path.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `resume-verdict`: replace the top-20 stack-match / must-have / LLM-coherence
  model with vacancy-coverage scoring, per-skill new-vacancy unlock, and an
  interactive role/filter selector; remove all LLM coherence and résumé-analysis
  persistence requirements.

## Impact

- **Backend (Go):** `internal/verdict` (reshape `Compute` to the coverage model;
  delete `coherence.go` + its tests), `internal/handler/resume_verdict.go`
  (single `GET` endpoint driven by facet params; drop `ResumeVerdict` POST and
  `applyStoredAnalysis`), `internal/handler/handler.go` (route wiring),
  `internal/search` (two facet queries: role total + uncovered distribution).
- **Database:** new migration dropping `search_profiles.resume_analysis`; regen
  sqlc.
- **Frontend (SvelteKit):** `web/src/routes/my/profiles/[id]/verdict/+page.svelte`
  (remove coherence upload; add filter panel), `VerdictView.svelte` (coverage +
  gaps layout), `SearchProfilesView.svelte` / `skillGap.ts` (headline coverage
  numbers), `$lib/api` + `$lib/types` (drop coherence calls/types), generated
  contracts.
- **Removed API surface:** `POST /me/profiles/:id/verdict`; the verdict response
  no longer carries `coherence`/`advice`/`analyzed_at`.
