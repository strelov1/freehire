## 1. Minting a vacancy with no external URL

- [x] 1.1 `job.IsDirectApply(source, externalID)` predicate and the `freehire:` prefix constant, with unit tests
- [x] 1.2 `moderation.CreateInput.ExternalID`: when set, `URL` must be empty and is not validated; `derive` uses it as the external id; unit tests for both modes and for submissions staying unchanged
- [x] 1.3 Audit every reader of `jobs.url` / `job.URL` (jobview `outboundurl.Tag`, liveness, ghost crosscheck, linksource, sitemap, OJCP, searchping) for the empty value; fix or record why each is unaffected
- [x] 1.4 `jobview` emits `direct_apply: true` (omitted otherwise) and an empty `url` for direct-apply vacancies; unit test

## 2. Employer questions

- [x] 2.1 `applyform.FreehireForm` (0–10 questions; label ≤ 300, type text/boolean/select, required; select 2–20 distinct non-empty options) with server-minted ids `q1…qN`; unit tests
- [x] 2.2 `employer.VacancyInput.ApplyMode` + `Questions`: `freehire` mints `freehire:<uuid>` with no URL (skipping the `Owner(url)` race guard) and writes the `apply_forms` row (provider `freehire`); `external` refuses questions and still requires a URL; integration tests
- [x] 2.3 Editing questions on an own vacancy (`VacancyPatch.Questions`), refusing a mode change and another employer's vacancy; integration tests
- [x] 2.4 HTTP: `employerVacancyRequest` carries `apply_mode` and `questions`; handler integration tests

## 3. The application

- [x] 3.1 Migration `direct_applications` (squawk clean) + queries `UpsertDirectApplication` (with the daily-cap CTE and the sent-guard), `MarkDirectApplicationSent`, `GetDirectApplication`; `make sqlc`
- [x] 3.2 `DIRECT_APPLY_DAILY_LIMIT` in `internal/platform/config` (default 30, strict); unit tests
- [x] 3.3 `directapply.ValidateAnswers` (required, boolean, select option, unknown id, note ≤ 2000); unit tests
- [x] 3.4 `directapply.Service.Apply`: the flow in design §4 behind `Mailer`, CV renderer and repository ports; unit tests with fakes for every refusal, the failed-send-stays-unsent case, the retry-replaces-content case and the side effects
- [x] 3.5 Add `internal/ingest/directapply` to `internal/platform/arch/layering/blocks.go`; package `AGENTS.md`; link it from the root module table
- [x] 3.6 Integration tests (`-tags=integration`) against Postgres: one row per (job, user), the conflict on a sent application, the cap, a retry not counting

## 4. Delivery

- [x] 4.1 `emailnotify` application mailer: `GroupEssential`, To = employer confirmed email, Reply-To = candidate, PDF attachment, questions/answers/note in HTML and text; unit test over the built `Message`
- [x] 4.2 Wire `directapply.Service` and the mailer in `handler.Register` (nil-safe port, 501 when Typst or SES is absent)

## 5. HTTP

- [x] 5.1 `GET /jobs/:slug/direct-apply`: company, questions, and for a signed-in user `prefill` (answer bank by topic) and `applied`; handler integration test
- [x] 5.2 `POST /jobs/:slug/direct-apply`: error mapping (400 naming the question, 403 unverified, 404, 409 sent or in flight, 429, 502 with no provider detail); handler integration test
- [x] 5.3 API docs: both endpoints in `web/src/lib/docs/api-spec.ts` (regenerate `docs/API*.md` and the OpenAPI references); `direct_apply` on the job in `web/static/openapi.yaml` (that file excludes signed-in routes by design, so only the field)

## 6. Web

- [x] 6.1 Types and API client for `direct_apply`, `apply_mode`, questions, the direct-apply endpoints
- [x] 6.2 `EmployerDashboard.svelte`: apply-mode choice (URL field only for external, and the url check only on create of an external vacancy — today an edit of a url-less vacancy cannot be saved) and the question editor; submit disabled while in flight
- [x] 6.3 `DirectApplyForm.svelte`: CV picker, typed answers with prefill, note, the "sent to <company>" consent line, error states; `JobView` opens it for `direct_apply` (no external link, no "did you apply?" prompt, no sign-in gate opening a blank tab) and shows "Application sent" when applied
- [x] 6.4 "Only on freehire" badge on the job page and job cards; `JobRow`'s Apply shortcut links to the job page for `direct_apply`
- [x] 6.5 `jobPostingJsonLd`: `directApply: true` only for `job.direct_apply`; unit test for both cases
- [x] 6.6 Privacy page paragraph on CVs reaching an employer only when the candidate sends them

## 7. Verify

- [x] 7.1 `gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, the tagged suites for touched packages, `pnpm check` in `web/`
- [ ] 7.2 Live check after deploy (a local run cannot deliver: it needs SES and Typst, and the server answers 501 without them): publish a `freehire` vacancy from a test employer account, apply as another user, confirm the email arrives with the CV and Reply-To, the tracking entry, the badge, and `directApply` in the page's JSON-LD. The flow itself is covered by `TestDirectApplyEndToEnd` with a fake mailer
