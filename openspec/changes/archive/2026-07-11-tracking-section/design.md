## Context

The personal-jobs area lives under `/my/jobs/*` (frontend: Board index, `pipeline`, `history`, with a shared `+layout.svelte` tab bar) and `/me/jobs/*` (backend: `ListMyJobs`, `MyPipeline`, `ListViewedSlugs`, `SwipeDeck` in `internal/handler/me_jobs.go`). The AI fit analysis (change `fit-analysis-quota`) already persists one row per `(user, job)` in `user_job_analysis` and exposes a per-user quota; the analysed jobs have no listing surface yet, and the section name "My jobs" is unclear.

The released **freehire-cli** calls `GET /api/v1/me/jobs` (`client.go` `MyJobs`), so the backend path is an external contract.

## Goals / Non-Goals

**Goals:**

- Add an AI-fit tab listing the caller's analysed jobs, with the quota counter.
- Rename the section to "Tracking" across UI, frontend URLs, and backend, coherently.
- Preserve every existing entry point: old frontend URLs redirect, old API paths alias.

**Non-Goals:**

- No schema migration (reads existing `user_job_analysis`).
- Not migrating the freehire-cli in this change (it keeps working on the alias; migrates on its own release).
- No change to the fit analysis chain, the quota rule, or the fit page itself.

## Decisions

### Analysed-jobs endpoint reuses the cache table + quota helpers

`ListUserJobAnalyses(user_id)` joins `user_job_analysis` → `jobs`, selecting slug/title/company/closed_at/content_hash plus the stored analysis blob and its stamps, ordered by `created_at DESC`. `ListMyAnalyses` parses each blob for `overall_score`/`verdict`, computes `stale` by reusing `stampsFresh` (with the caller's live CV upload time + current model), and returns `{data, meta:{quota}}` via the existing `fitQuotaFor`. No new domain concept — it's a projection of data we already store.

### Canonical `/me/tracking` with `/me/jobs` aliases (CLI safety)

Register the tracking handlers under `/me/tracking*` as canonical and re-register the same handler funcs under the legacy `/me/jobs*` paths. The alias set is a small, explicit block in `handler.go` with a comment naming the freehire-cli as the reason and marking migration as a seam. Handler file renamed `me_jobs.go`→`me_tracking.go`; methods renamed to the Tracking vocabulary (`ListTrackedJobs`, `TrackingPipeline`); `ListViewedSlugs`/`SwipeDeck` keep their names.

### Frontend rename via moved routes + a reroute redirect

Move `web/src/routes/my/jobs/*` → `my/tracking/*`, add the `analyses` tab in the shared layout, and update the label to "Tracking". Old URLs are handled by a `reroute` hook (`src/hooks.ts`) mapping `/my/jobs` and `/my/jobs/<rest>` → `/my/tracking/<rest>` — one central rule rather than a redirect stub per route. All internal `resolve('/my/jobs...')` references and `api.ts` paths switch to the new canonical paths; `api.myAnalyses()` calls `/me/tracking/analyses`.

### Analysed-list item shape

`{ slug, title, company, closed, overall_score, verdict, analysed_at, stale }` — a flat, list-oriented projection (not the full `jobfit.Analysis`), generated to TS so the page and `api.ts` share the type.

## Risks / Trade-offs

- **Dual API surface (accepted):** carrying `/me/jobs` aliases is mild duplication, justified by a real external consumer; removed once the CLI migrates. Documented inline as a seam.
- **Redirect correctness:** the `reroute` rule must not catch unrelated paths (only `/my/jobs` and its subpaths) and must preserve the trailing subpath and query. Covered by a unit test on the pure reroute function.
- **Stale computation cost:** the list computes `stale` per row, but that's pure comparison over already-fetched columns plus two per-request constants (CV time, model) — no extra I/O per row.
- **Churn:** renaming touches many files; kept mechanical and behavior-preserving, with tests (backend endpoint + reroute) guarding behavior.
