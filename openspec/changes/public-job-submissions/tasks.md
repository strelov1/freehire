## 1. Schema & DB access

- [x] 1.1 Add migration `migrations/0019_job_submissions.sql`: `job_submissions` table (`id`, `submitted_by` FK users ON DELETE CASCADE, `url`, `source`, `title`, `company`, `location`, `remote`, `description`, `posted_at`, `status` with CHECK in (`pending`,`approved`,`rejected`) default `pending`, `review_reason` default `''`, `reviewed_by` FK users ON DELETE SET NULL, `reviewed_at`, `job_id` FK jobs ON DELETE SET NULL, `created_at` default now()) + partial unique index on `lower(url) WHERE status='pending'`
- [x] 1.2 Add queries to `internal/db/queries/`: `CreateSubmission`, `GetSubmission` (by id; the approve/reject flow guards status in the service + Mark* are status-scoped, so no `FOR UPDATE` lock is held across the mint), `ListPendingSubmissions` (join users for submitter email), `ListSubmissionsByUser`, `MarkSubmissionApproved` (status/reviewed_by/reviewed_at/job_id), `MarkSubmissionRejected` (status/reviewed_by/reviewed_at/review_reason)
- [x] 1.3 Add `role` to the projection of the user read queries (the ones feeding `auth/me`)
- [x] 1.4 Run `make sqlc` and commit the regenerated `internal/db`

## 2. Reuse seam in moderation

- [x] 2.1 Export `moderation.CreateInput.Validate()` (rename the unexported `validate`), update internal callers; confirm `go test ./internal/moderation/` still passes

## 3. Submission service (`internal/submission`)

- [x] 3.1 Define `Service`, `Repository` interface, `Minter` seam, sentinel errors (`ErrSubmissionNotFound`, `ErrDuplicatePending`, `ErrAlreadyDecided`; validation reuses `moderation.ErrInvalid`) — mirror `internal/moderation`'s shape; submit content reuses `moderation.CreateInput`
- [x] 3.2 Implement `Submit(ctx, userID, CreateInput)`: validate via `moderation.CreateInput.Validate()`, persist `pending`; map the partial-unique violation to `ErrDuplicatePending`
- [x] 3.3 Implement `ListMine(ctx, userID)` and `ListPending(ctx)`
- [x] 3.4 Implement `Approve(ctx, reviewerID, id)`: load via `Get`, guard `status=='pending'` (else `ErrAlreadyDecided`), mint via the `Minter` (`moderation.Create(ctx, submittedBy, …)`), then `MarkSubmissionApproved(id, reviewerID, jobID)` (status-scoped → `ErrAlreadyDecided` on a concurrent decision)
- [x] 3.5 Implement `Reject(ctx, reviewerID, id, reason)`: load, guard `pending`, `MarkSubmissionRejected`
- [x] 3.6 Implement `QueriesRepository` adapting `*db.Queries` to `Repository`
- [x] 3.7 Unit tests with a `fakeRepo` + `fakeMinter`: submit validation, duplicate-pending → conflict, approve mints under the submitter + marks (all content fields incl. Description/Source forwarded), approve/reject of a decided row → `ErrAlreadyDecided`, reject records reason

## 4. HTTP layer

- [x] 4.1 Add `role` to `userResponse` + `toUserResponse` in `internal/handler/auth.go` (+ `Role` on `accounts.User` and the three repository read mappings)
- [x] 4.2 Add `internal/handler/submissions.go`: request/response shapes (omit raw submitter id on self response; `submitter_email` only on the moderator queue), a `submissionError` mapper (`moderation.ErrInvalid`→400, `ErrDuplicatePending`/`ErrAlreadyDecided`→409, `ErrSubmissionNotFound`→404), and handlers `CreateSubmission`, `ListMySubmissions`, `ListPendingSubmissions`, `ApproveSubmission`, `RejectSubmission`; reuse `createJobRequest.toCreateInput()`
- [x] 4.3 Wire the submission Service into `handler.Register` and add routes: `POST /submissions` + `GET /me/submissions` (`RequireAuthOrKey`); `GET /submissions`, `POST /submissions/:id/approve`, `POST /submissions/:id/reject` (`+ RequireRole("moderator")`)
- [x] 4.4 Integration tests (`//go:build integration`, `TestSubmissionsEndToEnd`) for the routes incl. role/auth gating and status codes (201/400/401/403/409), approve mints under the submitter, reject records the reason, my-submissions scoping, role on `/auth/me` — all 11 subtests green against real Postgres

## 5. Generated contracts & web

- [x] 5.1 `role`/`Submission`/`SubmissionInput` added by hand to `web/src/lib/types.ts` (User is hand-written; `cmd/gen-contracts` only emits Job/Enrichment+vocabs, so no `make gen-contracts` needed)
- [x] 5.2 Submissions API client in `web/src/lib/api.ts` (`submitJob`, `listMySubmissions`, `listPendingSubmissions`, `approveSubmission`, `rejectSubmission`; added to both the return object and the named-export destructuring)
- [x] 5.3 Web route `/submit` + `SubmitView.svelte`: authenticated job-submission form (url/title/company required; location/remote/description/source optional); guests prompted to sign in; 409/400 surfaced via ApiError
- [x] 5.4 Web route `/my/submissions` + `MySubmissionsView.svelte`: the caller's submissions with a status pill + rejection reason
- [x] 5.5 Web route `/moderation` + `ModerationView.svelte`: pending queue with Approve/Reject (reason via prompt); route render + UserMenu "Moderation" entry gated on `user.role === 'moderator'`
- [x] 5.6 Verified web: `svelte-check` 0 errors, SSR `npm run build` succeeds, new files clean under oxlint (eslint `no-navigation-without-resolve` is the documented pre-existing project-wide red baseline; new links match existing href style)
- [x] 5.7 Public recruiter-facing landing `/recruiters` + `RecruitersView.svelte` (pitch + how-it-works + CTAs to `/submit`); "For recruiters" link added to the header nav (`TopBar.svelte`) and the page added to `sitemap.xml` (indexable, unlike the noindex personal pages)

## 6. CLI (`../freehire-cli`, separate repo)

- [x] 6.1 `freehire submit --url --title --company [--location --remote --description --source]` → `POST /submissions` (reuses `client.CreateJobParams`); TDD'd via `submissions_test.go`
- [x] 6.2 `freehire submissions` → `GET /me/submissions`, rendering id/status/title/company (+ reason when rejected)
- [x] 6.3 Moderator subcommands: `submissions pending` (`GET /submissions`, shows submitter email), `submissions approve <id>`, `submissions reject <id> --reason`; all 5 CLI tests + client suite green, vet/gofmt clean
- [ ] 6.4 Release per `freehire-cli` ops (raw binaries + gh release; keep the self-hosted installer in sync) — deferred to ship time (outward-facing publish, separate repo; pairs with 7.2 deploy)

## 7. Validate & ship

- [x] 7.1 Verified: `go build`/`go vet`/`gofmt` clean; full `go test ./...` green; handler integration suite green (incl. `TestSubmissionsEndToEnd`, 188s); `openspec validate --strict` valid; web `svelte-check` 0 errors + SSR build; CLI `go test ./...` green
- [ ] 7.2 Deploy from `origin/main`, then run migration `0019` manually against prod (`make reindex` not required); release the `freehire-cli` commands (6.4) — deferred to ship time (outward-facing; needs your go-ahead)
