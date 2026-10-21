## 1. Minting a vacancy with no external URL

- [ ] 1.1 `job.IsDirectApply(source, externalID)` predicate and the `freehire:` prefix constant, with unit tests
- [ ] 1.2 `moderation.CreateInput.ExternalID`: when set, `URL` must be empty and is not validated; `derive` uses it as the external id; unit tests for both modes and for submissions staying unchanged
- [ ] 1.3 Audit every reader of `jobs.url` / `job.URL` (jobview `outboundurl.Tag`, liveness, ghost crosscheck, linksource, sitemap, OJCP, searchping) for the empty value; fix or record why each is unaffected
- [ ] 1.4 `jobview` emits `apply_mode` (`external` | `freehire`) and an empty `url` for direct-apply vacancies; unit test

## 2. Employer questions

- [ ] 2.1 `directapply.ValidateQuestions` (0–10 questions; label, type text/boolean/select, required; select 2–20 distinct options) and server-minted ids `q1…qN`; unit tests
- [ ] 2.2 `employer.VacancyInput.ApplyMode` + `Questions`: `freehire` mints `freehire:<uuid>` with no URL and writes the `apply_forms` row (provider `freehire`); `external` refuses questions and still requires a URL; integration tests
- [ ] 2.3 Editing questions on an own vacancy (`VacancyPatch.Questions`), refusing a mode change and another employer's vacancy; integration tests
- [ ] 2.4 HTTP: `employerVacancyRequest` carries `apply_mode` and `questions`; handler integration tests

## 3. The application

- [ ] 3.1 Migration `direct_applications` (squawk clean) + queries `UpsertDirectApplication` (with the daily-cap CTE and the sent-guard), `MarkDirectApplicationSent`, `GetDirectApplication`; `make sqlc`
- [ ] 3.2 `DIRECT_APPLY_DAILY_LIMIT` in `internal/platform/config` (default 30, strict); unit tests
- [ ] 3.3 `directapply.ValidateAnswers` (required, boolean, select option, unknown id, note ≤ 2000); unit tests
- [ ] 3.4 `directapply.Service.Apply`: the flow in design §4 behind `Mailer`, CV renderer and repository ports; unit tests with fakes for every refusal, the failed-send-stays-unsent case, the retry-replaces-content case and the side effects
- [ ] 3.5 Add `internal/ingest/directapply` to `internal/platform/arch/layering/blocks.go`; package `AGENTS.md`; link it from the root module table
- [ ] 3.6 Integration tests (`-tags=integration`) against Postgres: one row per (job, user), the conflict on a sent application, the cap, a retry not counting

## 4. Delivery

- [ ] 4.1 `emailnotify` application mailer: `GroupEssential`, To = employer confirmed email, Reply-To = candidate, PDF attachment, questions/answers/note in HTML and text; unit test over the built `Message`
- [ ] 4.2 Wire `directapply.Service` and the mailer in `handler.Register` (nil-safe port, 501 when Typst or SES is absent)

## 5. HTTP

- [ ] 5.1 `GET /jobs/:slug/direct-apply`: company, questions, and for a signed-in user `prefill` (answer bank by topic, screening-answer fallback) and `applied`; handler integration test
- [ ] 5.2 `POST /jobs/:slug/direct-apply`: error mapping (400 naming the question, 403 unverified, 404, 409, 429, 502); handler integration test
- [ ] 5.3 `web/static/openapi.yaml`: both endpoints and the employer fields

## 6. Web

- [ ] 6.1 Types and API client for `apply_mode`, questions, the direct-apply endpoints
- [ ] 6.2 `EmployerDashboard.svelte`: apply-mode choice (URL field only for external) and the question editor; submit disabled while in flight
- [ ] 6.3 `DirectApplyForm.svelte`: CV picker, typed answers with prefill, note, the "sent to <company>" consent line, error states; `JobView` opens it for `freehire` and shows "Application sent" when applied
- [ ] 6.4 "Only on freehire" badge on the job page and job cards
- [ ] 6.5 `jobPostingJsonLd`: `directApply: true` only for `apply_mode === 'freehire'`; unit test for both cases
- [ ] 6.6 Privacy page paragraph on CVs reaching an employer only when the candidate sends them

## 7. Verify

- [ ] 7.1 `gofmt -l .`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`, the tagged suites for touched packages, `pnpm check` in `web/`
- [ ] 7.2 Run the app locally: publish a `freehire` vacancy, apply as another user, see the email payload (log transport) and the tracking entry
