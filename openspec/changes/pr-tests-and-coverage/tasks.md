## 1. Gap-fill: résumé / Profile API

- [x] 1.1 Extend `GetResume` / fakeBank tests so SeedHistory projects appear under `structured.projects` (not only experience)
- [x] 1.2 Add a placeless-highlights case on `GET /me/resume` (empty place fields + highlights)
- [x] 1.3 Confirm or extend `internal/resume` + handler tests for missing-object → `ErrNotStored` → retry `409` with upload-again messaging
- [x] 1.4 Run `go test ./internal/handler/ ./internal/resume/` and `go vet -tags=integration ./internal/handler/ ./internal/resume/`

## 2. Gap-fill: experience promote-to-project

- [x] 2.1 Add a store (or handler) test: create `kind=project` employment, update unplaced atom with that id, assert bank read groups it under the project
- [x] 2.2 Reuse or assert the existing “cannot attach to another owner’s employment” path still fails
- [x] 2.3 Run `go test ./internal/experience/ ./internal/handler/` for the new cases

## 3. Optional SPA unit coverage

- [x] 3.1 Skipped — no cheap Svelte component harness; Profile behaviour covered via API tests
- [x] 3.2 Skipped — MatchAnalysis idle Run needs heavy stream mocking; leave for a later UI harness

## 4. Go coverage tooling

- [x] 4.1 Add `make cover` (unit `-coverprofile`) and `make cover-html` (or equivalent) documenting paths under `/tmp` or `coverage/`
- [x] 4.2 Add `make cover-integration` for tagged suite with its own profile (document Docker need)
- [x] 4.3 Wire CI backend job to write coverprofiles and `upload-artifact` them (no % fail)

## 5. Web coverage tooling

- [x] 5.1 Add `@vitest/coverage-v8` and `test:coverage` script in `web/`
- [x] 5.2 Ensure `coverage/` is gitignored
- [x] 5.3 Wire CI web job to run coverage (or unit + coverage) and upload the report artifact (no % fail)

## 6. Verify

- [x] 6.1 Run unit + integration (`go test ./...`, `go test -tags=integration ./...` or the cheap vet line + targeted integration) for touched packages
- [x] 6.2 Confirm local cover commands produce readable reports
- [x] 6.3 Confirm CI config is valid YAML and does not introduce a coverage threshold
