## 1. Storage schema & DB access

- [x] 1.1 Add migration `0011_resume_structured.sql`: `ALTER TABLE users ADD COLUMN resume_structured jsonb, ADD COLUMN resume_structured_model text, ADD COLUMN resume_structured_uploaded_at timestamptz` (all nullable)
- [x] 1.2 Add sqlc queries in `internal/db/queries/users.sql`: set the three structured columns (stamp with model + given uploaded_at), read them (extend the résumé pointer read), and clear them; extend `ClearUserResume` to also null the structured columns
- [x] 1.3 Run `make sqlc` and commit the regenerated `internal/db`

## 2. Structured-résumé contract & extraction (`internal/resumeextract`)

- [x] 2.1 Define the typed `Structured` contract (contacts, summary, work-experience entries with title/company/dates, education, languages, links, total years) with JSON tags
- [x] 2.2 Implement `Structured.Sanitize()` — bound every string length, cap each array's cardinality, coerce total-years to a non-negative bounded int, drop empty entries; unit-test the bounds and the drop-empty behavior (RED first)
- [x] 2.3 Implement `Extractor` over `*llm.Client` with `Extract(ctx, cvText) (Structured, error)` using `GenerateJSON`, returning the sanitized value; the extractor is disabled (nil client) → returns a sentinel/zero so callers skip. Unit-test with a fake LLM (parse + sanitize path, and the unconfigured path)

## 3. Résumé storage side effect (`internal/resume`)

- [x] 3.1 Add `Store.SetStructured(ctx, userID, s Structured, model string, uploadedAt time.Time)` and `Store.Structured(ctx, userID) (Structured, bool)` that serves only when the stamp matches the current résumé upload time (else `ok=false`); unit-test the stale-mismatch → absent rule with a fake repo (RED first)
- [x] 3.2 Ensure `Store.Delete` clears the structured columns (via the extended `ClearUserResume`); unit-test that delete clears the structure

## 4. Handler wiring — extraction & read surface

- [x] 4.1 Add `a.structuredExtractor` (built from `cfg.LLM.WithTimeout(...)`, nil when unconfigured) to the `API` struct and `Register`
- [x] 4.2 Add `a.extractStructuredResume(userID, text string)` — background goroutine on its own timeout context, best-effort (log without CV bytes, swallow), persisting via `Store.SetStructured`; wire `go a.extractStructuredResume(...)` beside `go a.embedResume(...)` in BOTH `PutResume` and `ExtractResumeProfile`
- [x] 4.3 Extend `resumeMetaResponse` (and `newResumeMeta`) with an optional `structured` field, populated from `Store.Structured` (null when absent/stale/unconfigured); update `GetResume`. Handler test asserts present vs null shapes

## 5. Fit-analysis consumption

- [x] 5.1 Add `StructuredResume string` to `jobfit.Input` and include it in the Stage-1 prompt as pre-normalized context beside `CVText` (never replacing it); unit-test that an empty value produces today's prompt behavior
- [x] 5.2 In `PostJobFit` and `job_fit_stream.go`, load the caller's current structured résumé (JSON) and pass it into `Input.StructuredResume`; degrade to empty when absent

## 6. Contract generation

- [x] 6.1 Register `resumeextract.Structured` in `cmd/gen-contracts`, regenerate the TS types, and commit

## 7. Frontend (read-only profile rendering)

- [x] 7.1 Render the structured résumé sections (experience, education, contacts, languages, links, summary) read-only on the profile page from the résumé status response; omit the section entirely when null
- [x] 7.2 Web verify: `svelte-check` (0 errors) + `vitest` (141 pass). Lint: only the pre-existing `no-navigation-without-resolve` external-link category (same as GithubStars/JobView); not CI-gated. Live visual check pending a running stack.

## 8. Verification

- [x] 8.1 `go build ./... && go vet ./... && go test ./...` green (66 packages, 0 failures)
- [~] 8.2 Verified via the test suite: extractor with a fake LLM (parse+sanitize+unconfigured), Store staleness (fresh/stale/absent + delete-clears), the monotonic SQL write-guard (generated), GetResume present/null shapes, Stage-1 prompt inclusion/omission, and the frontend (svelte-check + vitest). Live end-to-end (real LLM + S3 + browser) NOT run — requires prod LLM creds and S3; degradation with no LLM is unit-covered.
- [x] 8.3 Manual-prod-apply note carried in `migrations/0011_resume_structured.sql` (lines 12-13), per the migrations convention (deploy runs via ../freehire-ops)
