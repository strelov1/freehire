## Why

"My jobs" is a confusing umbrella: it already holds saved jobs (Board), the application Pipeline, and view History, and we're adding a fourth surface — the jobs a user has run AI fit analysis on. The label reads as "jobs I posted" and doesn't convey "the jobs I'm tracking". Renaming the section to **Tracking** and adding an **AI fit** tab makes the mental model clear, and the new tab becomes the natural home for the monthly fit-analysis quota counter (today only shown contextually).

## What Changes

- Add an **AI fit** tab listing the jobs the caller has analysed (title, company, overall score, verdict, analysed date, a stale flag, and a **Closed** badge for closed jobs), newest first, each linking to its fit page. A **quota banner** ("N/10 used this month") sits above the list.
- New endpoint `GET /api/v1/me/tracking/analyses` returning the analysed jobs with the caller's `quota` in `meta`.
- **Rename the section "My jobs" → "Tracking"** across the UI, the frontend URLs, and the backend:
  - Frontend routes `/my/jobs/*` → `/my/tracking/*` (Board index, Pipeline, History, + the new AI fit tab); old `/my/jobs/*` URLs **308-redirect** to the new paths so bookmarks/links keep working.
  - Backend: routes move to `/me/tracking/*`; the old `/me/jobs/*` routes are **removed** (no alias). Handler file `internal/handler/me_jobs.go` → `me_tracking.go`, handler methods renamed to the Tracking vocabulary.
- **BREAKING:** `/me/jobs*` is gone. The freehire-cli (which calls `GET /me/jobs`) is updated to `/me/tracking` in the same pass, but already-installed CLI binaries break until re-released/reinstalled.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `user-job-tracking`: adds the analysed-jobs list endpoint (with quota in meta), renames the tracking surface to `/me/tracking` (retaining `/me/jobs` aliases), and renames the frontend `/my/jobs` section to `/my/tracking` with redirects from the old URLs.

## Impact

- **Backend**: `internal/handler/me_jobs.go`→`me_tracking.go` (methods renamed), `internal/handler/handler.go` (route rename + `/me/jobs` aliases + new `/me/tracking/analyses`), new `ListMyAnalyses` handler, new `internal/db/queries` `ListUserJobAnalyses`, regenerated `internal/db`. Reuses `stampsFresh`/quota helpers from `job_fit.go`.
- **API**: canonical `/me/tracking/*` (old `/me/jobs/*` aliased); new `GET /me/tracking/analyses`. api-docs page updated.
- **Frontend** (`web/`): routes moved to `my/tracking/*`, new `analyses` tab, label "My jobs"→"Tracking", redirects from `/my/jobs/*`, `api.ts` paths + `myAnalyses()`, and all `resolve('/my/jobs...')` references (TopBar, HeaderMenu, HomeView, CliView, swipe/recommendations loaders).
- **External**: freehire-cli keeps working via the `/me/jobs` aliases; it should migrate to `/me/tracking` on its next release.
- **No schema migration** — reads the existing `user_job_analysis` table.
