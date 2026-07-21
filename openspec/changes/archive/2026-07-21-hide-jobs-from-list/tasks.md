## 1. Backend — dismissed-slugs cross-reference endpoint

- [x] 1.1 Add `ListDismissedJobSlugs` query to `internal/db/queries/user_jobs.sql` (public_slug where `dismissed_at IS NOT NULL`, scoped to user), mirroring `ListSavedJobSlugs`; regenerate sqlc (`make sqlc`)
- [x] 1.2 Add `DismissedSlugs` method to the tracking service, mirroring `SavedSlugs`
- [x] 1.3 Add `ListDismissedSlugs` handler + wire `GET /me/tracking/dismissed` route in `internal/handler/handler.go`, mirroring `ListSavedSlugs`; cover with an integration test asserting the caller-scoped `{"data":[slug,...]}` shape

## 2. Backend — `dismissed` filter on the tracking list

- [x] 2.1 Add the `dismissed` branch to `ListUserJobs` (`filter = 'dismissed' AND dismissed_at IS NOT NULL`) and a `dismissed` count to `CountUserJobs`; regenerate sqlc
- [x] 2.2 Thread the `dismissed` filter value through the tracking service / `MyJobsFilter` vocabulary and the `/me/tracking` list handler; extend the tracking integration test to assert a dismissed row lists under `filter=dismissed` and is excluded from `saved`/`viewed`

## 3. Frontend — API client + types

- [x] 3.1 Add `listDismissedSlugs()` to `web/src/lib/api.ts` (mirror `listSavedSlugs`) and add `'dismissed'` to the `MyJobsFilter` type in `web/src/lib/types.ts`
- [x] 3.2 N/A: api layer has no unit-testable seam beyond the URL string; covered by the integration tests in 1.3/2.2

## 4. Frontend — dismissed slugs store

- [x] 4.1 Create `web/src/lib/dismissedJobs.svelte.ts` mirroring `savedJobs.svelte.ts` (`isDismissed`/`markDismissed`/`markUndismissed`/`ensureDismissedLoaded`), loading from `api.listDismissedSlugs`
- [x] 4.2 N/A: no saved/viewed store tests exist to mirror (thin reactive Set glue, no branching logic); verified via svelte-check + visual QA

## 5. Frontend — hide control on the job card

- [x] 5.1 Add an `EyeOff` hover-revealed hide control to `web/src/lib/components/JobRow.svelte` (sibling of the card link, bottom corner), with optimistic `markDismissed` + `api.dismissJob`, rollback on failure, and sign-in routing for signed-out clicks — mirroring `toggleSave`
- [x] 5.2 Emit a hide event/callback from `JobRow` so the parent feed can surface undo (prop callback, default no-op so embedded uses are unaffected)

## 6. Frontend — feed exclusion + undo toast

- [x] 6.1 In `web/src/lib/components/JobsView.svelte`, `ensureDismissedLoaded()` on mount for signed-in users and filter `jobs.items` by `!isDismissed(slug)`
- [x] 6.2 Add a local, self-contained "Job hidden — Undo" toast (fixed-position, timeout) wired to `JobRow`'s hide callback; Undo calls `api.undismissJob` + `markUndismissed`

## 7. Frontend — Activity "Hidden" tab

- [x] 7.1 Add a fourth "Hidden" tab to `web/src/routes/my/activity/+layout.svelte` (route, aria, active-state) and create `web/src/routes/my/activity/hidden/+page.svelte`
- [x] 7.2 Create `web/src/lib/components/Hidden.svelte` mirroring `SavedJobs.svelte` (`api.listMyJobs('dismissed', …)`), with a per-row un-hide action (`api.undismissJob` + remove from list + `markUndismissed`) and an empty state

## 8. Verification

- [x] 8.1 go build/vet/test all green; dismissed + tracking integration tests pass under -tags=integration
- [x] 8.2 Live browser verify against my server (:8091) + dev DB: hide/vanish/toast, signed-out→sign-in, undo-restore, toast auto-dismiss, Hidden tab list + un-hide→empty; all green
