## Context

The verdict feature (capability `resume-verdict`) currently layers three things
on a search profile: a deterministic "stack match" (share of the top-20 in-demand
skills the profile holds), a must-have designation, and an optional LLM
"coherence" score that requires a second résumé upload on the verdict page. The
deterministic core lives in `internal/verdict/verdict.go` (pure `Compute`), the
LLM layer in `internal/verdict/coherence.go`, and the HTTP surface in
`internal/handler/resume_verdict.go` (`GET` + `POST /me/profiles/:id/verdict`).
Market data comes from Meilisearch facet counts filtered to the profile's
specialization categories.

Users reason in vacancies, not skill-list ratios, and the coherence card reads as
a disconnected metric. This change reshapes the verdict into a vacancy-coverage
tool and removes the LLM coherence feature. The search filter machinery already
exists and is reusable: `search.FilterFromValues(url.Values)` builds a
Meilisearch filter from the same facet params `/jobs` uses, and
`Client.FacetCounts` returns both the filtered total (`EstimatedTotalHits`) and a
per-value facet distribution.

## Goals / Non-Goals

**Goals:**
- Coverage = count and percent of open role vacancies that list ≥1 profile skill.
- Per missing skill: the number of *new* (currently-uncovered) vacancies it
  unlocks, ranked biggest-win-first.
- An interactive, sidebar-style filter on the verdict page that recomputes
  coverage for an ad-hoc role without mutating the stored profile.
- Profile list shows the headline coverage number; verdict page shows the full
  breakdown.
- Keep `internal/verdict.Compute` a pure, I/O-free, unit-tested function.

**Non-Goals:**
- Any AI/LLM in the verdict path (removed outright).
- Touching the profile-form résumé→skills extraction (`POST /me/resume/extract`)
  or the résumé-storage subsystem it shares — out of scope, left intact.
- Per-vacancy "match score" or weighting by how many skills overlap — coverage is
  binary (a vacancy is covered if it shares at least one skill).
- A new single-profile detail route — the existing list cards carry the headline.

## Decisions

### Decision 1: Compute coverage with exactly two facet queries

For a role filter `R` and the profile's skills `U = {u1..un}`:

- **Query A** — `FacetCounts(filter=R, facets=[skills])`
  → `total = A.Total` (all open role vacancies) and the gross demand
  distribution `A.Facets["skills"]` (to know which skills exist / rank the
  candidate gap set).
- **Query B** — `FacetCounts(filter = R AND skills != u1 AND … AND skills != un, facets=[skills])`
  → `uncovered = B.Total` and, for every skill `S`, `B.Facets["skills"][S]` =
  the number of *uncovered* vacancies listing `S` = the **new vacancies** `S`
  unlocks.

Then `covered = total − uncovered`, `coverage_percent = round(covered/total×100)`,
and each gap `S` (in `A.Facets`, not in `U`) carries `new_vacancies =
B.Facets["skills"][S]`, `unlock_percent = round(new_vacancies/total×100)`, ranked
by `new_vacancies` desc then slug asc, capped at 20.

*Why:* Meilisearch's facet distribution counts, within the matched set, how many
documents carry each value — so the single "uncovered" query yields the marginal
unlock for *every* skill at once. This is O(2) engine round-trips regardless of
gap count.

*Alternatives considered:* (a) one query per candidate skill for its marginal
unlock — correct but O(N) round-trips; rejected as needlessly chatty. (b) Keep
the top-20 gross-demand model and just annotate counts — cheaper (one query) but
does not answer "new vacancies unlocked"; rejected as it misses the point of the
redesign.

### Decision 2: `skills != u` per skill as its own AND group

"Vacancies listing none of the user's skills" is `skills != u1 AND … AND skills
!= un` (array non-membership is per-value). This maps cleanly onto the existing
`search.Filter(groups...)` shape (each `Neq("skills", u)` is a one-element AND
group), the same pattern `FilterFromValues` already uses for `*_exclude` facets.
No new filter primitive is needed — reuse `search.Neq` and `search.Filter`.

*Risk to check:* the AND-of-NEQs group must combine with the role filter `R`
(which is itself the nested `[][]string` from `FilterFromValues`). The handler
assembles the combined filter (append the NEQ groups to `R`'s groups) rather than
nesting two `any` filters. Because `FilterFromValues` returns `[][]string` (or
nil), the handler builds the role groups and the exclusion groups together and
passes them to `search.Filter` once.

### Decision 3: `Compute` stays pure; the handler does I/O

`internal/verdict.Compute` is reshaped to take the two facet results (raw: role
total + gross skill set, uncovered total + uncovered distribution) plus the
profile skills, and return the coverage `Verdict`. All Meilisearch calls and
filter assembly live in `resume_verdict.go`. This preserves the existing testing
seam (Compute is unit-tested with no engine) and mirrors how `computeVerdict`
already isolates the facet fetch.

New `Verdict` shape (JSON, regenerated to TS via `cmd/gen-contracts`):

```
Verdict {
  total            int64
  covered          int64
  coverage_percent int
  gaps: [ { name string; new_vacancies int64; unlock_percent int } ]
  // optional: skills_present []string for the "your skills" chips
}
```

`stack_match`, `must_have_*`, `coherence`, `advice`, `analyzed_at`, and the full
top-20 `skills` breakdown are removed.

### Decision 4: Single GET endpoint driven by facet params

`GET /me/profiles/:id/verdict` accepts the job-search facet params. The handler
takes the request query, and if it carries no `category`, injects the profile's
specializations as `category` values before `FilterFromValues`. The profile's
skills are read from the profile row, never from the query. `POST
/me/profiles/:id/verdict` and `applyStoredAnalysis` are deleted.

### Decision 5: Drop `search_profiles.resume_analysis` via migration

A new migration `migrations/00NN_drop_search_profiles_resume_analysis.sql` runs
`ALTER TABLE search_profiles DROP COLUMN IF EXISTS resume_analysis;` Regenerate
sqlc so `db.SearchProfile` loses the field and `SetResumeAnalysis` is removed
from the queries. Per the repo's migration convention this is initdb-only for
fresh volumes and a manual apply on prod (no runner yet) — note it in the finish
step.

### Decision 6: Reuse the existing filter UI on the verdict page

The verdict page mounts the same filter component the jobs list uses
(`FiltersPanel`), bound to the page's query params, and refetches the verdict on
change. Coverage state is keyed off the resolved params so back/forward and
direct links work (following the existing URL-synced-filter pattern). The profile
list keeps computing its headline via the verdict endpoint (or a shared client
helper) over each profile's own specializations.

## Risks / Trade-offs

- **Binary coverage overstates "fit"** (one shared skill ⇒ covered) → Mitigation:
  label it honestly ("vacancies mentioning at least one of your skills"), and the
  gap list's new-vacancy framing shows where real headroom is. A weighted match
  is a deliberate non-goal for this iteration.
- **`resume_analysis` drop is destructive and prod has no migration runner** →
  Mitigation: `DROP COLUMN IF EXISTS`, apply manually on prod before/with deploy;
  the column holds only re-derivable coherence output, so no data of value is
  lost.
- **Removing the `POST` endpoint + coherence types is BREAKING for the SPA** →
  Mitigation: ship backend and frontend together; regenerate contracts so the
  TS types drop the removed fields and the build fails loudly on any stale use.
- **Filter combines two `[][]string` sources** → Mitigation: assemble role groups
  and exclusion groups in the handler and call `search.Filter` once; cover the
  combined-filter shape with a unit test.
- **Verdict page filter divergence from `/jobs`** → Mitigation: reuse
  `FiltersPanel` rather than a bespoke panel, so the facet vocabulary stays in one
  place.

## Migration Plan

1. Backend: reshape `verdict.Compute` + `resume_verdict.go`, delete
   `coherence.go`, drop the `POST` route, remove LLM analyzer wiring from the
   verdict path (leave `internal/llm` and résumé storage intact).
2. DB: add the drop-column migration; `make sqlc`; commit generated code.
3. Contracts: regenerate Go→TS contracts; update `$lib/api`, `$lib/types`.
4. Frontend: verdict page (remove coherence UI, add `FiltersPanel`), rewrite
   `VerdictView`, update profile-list headline.
5. Deploy: apply the migration on prod manually, then deploy backend+frontend
   together. Rollback = redeploy the prior image; the dropped column is
   re-addable but empty (coherence is re-derivable, so acceptable).

## Open Questions

- None blocking. Confirmed during brainstorming: remove coherence entirely,
  vacancy-coverage metric, interactive on-page filter.
