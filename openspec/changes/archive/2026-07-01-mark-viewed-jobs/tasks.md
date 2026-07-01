## 1. Backend — viewed-slugs endpoint

- [x] 1.1 Add `ListViewedJobSlugs` query to `internal/db/queries/user_jobs.sql` (`SELECT jobs.public_slug FROM user_jobs uj JOIN jobs ON jobs.id = uj.job_id WHERE uj.user_id = $1`) and regenerate sqlc; verify `go build ./...` compiles with the generated method.
- [x] 1.2 Implement `ListViewedSlugs` handler in `internal/handler/me_jobs.go` returning `{"data": [slug, ...]}`, and register `GET /api/v1/me/jobs/viewed` under `RequireAuthOrKey` in `internal/handler/handler.go`. Cover with an integration test (`//go:build integration`): returns the caller's slugs only, `[]` when none, `401` unauthenticated.

## 2. Frontend — viewed-slugs data layer

- [x] 2.1 Add `listViewedSlugs(): Promise<string[]>` to `web/src/lib/api.ts` calling `GET /api/v1/me/jobs/viewed` and returning `data`.
- [x] 2.2 Add a reactive viewed-jobs store `web/src/lib/viewedJobs.svelte.ts`: a `$state` `Set<string>`, `hasViewed(slug)`, `markViewed(slug)`, and `ensureViewedLoaded()` that fetches once (guarded against repeat loads) and is a no-op for signed-out users.

## 3. Frontend — wire into the UI

- [x] 3.1 In `JobsView.svelte`, call `ensureViewedLoaded()` on mount when the user is authenticated (covers both list and search, which render through this component).
- [x] 3.2 In `JobRow.svelte`, add a `dimViewed = true` prop and dim the card when viewed: `class:opacity-60={dimViewed && hasViewed(job.public_slug)}` plus `hover:opacity-100`.
- [x] 3.3 In `JobView.svelte`, call `markViewed(slug)` after `recordJobView` resolves so the card is dimmed on back-navigation without a reload.
- [x] 3.4 Pass `dimViewed={false}` from the My Jobs surfaces (`JobHistory.svelte` and any board `JobRow` usage) so already-viewed-by-definition lists are not uniformly dimmed.

## 4. Verification

- [x] 4.1 Backend: `go build ./... && go vet ./...`, `go test ./...`, and the new handler integration test (`go test -tags=integration ./internal/handler/`) all green.
- [x] 4.2 Frontend: `npm run check` (svelte-check) and `npm run build` green; manually confirm viewed cards dim for a signed-in user and not for an anonymous visitor.
