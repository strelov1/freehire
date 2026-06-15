## 1. Schema & generated DB access

- [x] 1.1 Add `migrations/0020_job_reports.sql`: `job_reports` table (`id`, `reported_by`
  FK→users ON DELETE CASCADE, `job_id` FK→jobs ON DELETE CASCADE, `reason` TEXT with
  `CHECK (reason IN ('no_response','not_relevant','spam','fraud','other'))`, `details`
  TEXT NOT NULL, `contact_telegram` TEXT, `status` TEXT NOT NULL DEFAULT 'pending' with
  `CHECK (status IN ('pending','resolved','dismissed'))`, `review_reason` TEXT,
  `reviewed_by` FK→users ON DELETE SET NULL, `reviewed_at` TIMESTAMPTZ, `created_at`
  TIMESTAMPTZ NOT NULL DEFAULT now()), plus partial unique index
  `UNIQUE (reported_by, job_id) WHERE status = 'pending'`.
- [x] 1.2 Add `internal/db/queries/reports.sql`: `CreateReport` (insert pending, RETURNING *),
  `GetReport` (by id), `ListPendingReports` (join users for `reporter_email` + jobs for
  `job_slug`/`job_title`, newest first), `MarkReportResolved` (scoped `status='pending'`,
  set reviewed_by/reviewed_at, RETURNING *), `MarkReportDismissed` (scoped `status='pending'`,
  set review_reason/reviewed_by/reviewed_at, RETURNING *), and `CloseJobByID`
  (`UPDATE jobs SET closed_at=now(), updated_at=now() WHERE id=$1 AND closed_at IS NULL`).
- [x] 1.3 Run `make sqlc` (or `sqlc generate`) and commit the regenerated `internal/db` code.

## 2. Report domain package (`internal/report`)

- [x] 2.1 Define the package: `Report` sentinels (`ErrReportNotFound`, `ErrDuplicateOpen`,
  `ErrAlreadyDecided`, `ErrInvalid`), the reason vocabulary + validation, the `Repository`
  interface, and the `JobCloser` seam (`Close(ctx, jobID) error`).
- [x] 2.2 Implement `Service.File` (validate reason + non-empty/length-bounded details →
  `repo.Create`), `Service.ListPending`, `Service.Resolve` (load → guard pending →
  optionally `closer.Close(jobID)` → `MarkResolved`), `Service.Dismiss` (load → guard
  pending → `MarkDismissed`). Unit-test against an in-memory fake Repository/JobCloser
  (mirror `internal/submission/submission_test.go`): valid/invalid reason, blank details,
  duplicate-open, not-found, already-decided, resolve-with/without-close.
- [x] 2.3 Implement `QueriesRepository` adapting `*db.Queries`: map unique violation →
  `ErrDuplicateOpen`, `pgx.ErrNoRows` on get → `ErrReportNotFound`, no-row on a scoped
  mark → `ErrAlreadyDecided`; `Close` calls `CloseJobByID`.

## 3. HTTP handler & routes

- [x] 3.1 Add `internal/handler/reports.go`: `reportResponse` wire shape (omit
  `reported_by`; queue adds `reporter_email`/`job_slug`/`job_title`), `reportError` status
  mapping (`404`/`409`/`400`), and handlers `CreateReport` (resolve slug via
  `GetJobIDBySlug` → `404` on miss → `report.File` → `201`), `ListPendingReports`,
  `ResolveReport` (parse `{close_job bool}`), `DismissReport` (parse optional `{reason}`).
- [x] 3.2 Wire `a.report` in `Register` (construct `report.New(report.NewQueriesRepository(queries))`)
  and add routes: `POST /jobs/:slug/reports` (keyAuth), `GET /reports` +
  `POST /reports/:id/resolve` + `POST /reports/:id/dismiss` (keyAuth + requireModerator).
- [x] 3.3 Add `internal/handler/reports_integration_test.go` (`//go:build integration`,
  mirror `submissions_integration_test.go`): file→201, unauth→401, unknown slug→404,
  bad reason/blank details→400, duplicate-open→409, queue lists with reporter email +
  job fields, non-moderator→403, resolve(+close)→job closed, resolve(no close)→job open,
  dismiss records reason, decided→409.

## 4. Web (SvelteKit)

- [x] 4.1 Add the reports API client to `web/src/lib/api.ts` (`reportJob`,
  `listPendingReports`, `resolveReport`, `dismissReport`) and a reason→label map mirroring
  the five backend values.
- [x] 4.2 Add the "Пожаловаться на вакансию" button + two-step modal to `JobView.svelte`
  (step 1 reason picker, step 2 required details + optional Telegram → `reportJob`);
  signed-out click opens the auth dialog via `openAuthDialog`. Show a success/closed state
  after submit.
- [x] 4.3 Add the reports section to the `/moderation` route: pending queue with reporter
  email, job link, reason/details, and resolve (with a "close job" toggle) + dismiss
  actions, gated on `user.role === 'moderator'`.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./...`; recompile the build-tagged
  handler tests (`go test -tags=integration -run XXNONE ./internal/handler/`) and run the
  report integration test.
- [x] 5.2 Web: `npm run check` (svelte-check) + lint; manual smoke of the modal and the
  moderation queue (resolve-with-close removes the job from the list).
- [x] 5.3 Run `openspec validate report-job --strict` and confirm it passes.
