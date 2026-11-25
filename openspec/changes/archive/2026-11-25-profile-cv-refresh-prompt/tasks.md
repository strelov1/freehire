## 1. Base reseed API

- [x] 1.1 Add cookie-only `POST /api/v1/me/cvs/base/reset-from-resume` (or equivalent) that reseeds/creates the base CV from `seedSource()` via existing `reseedBaseFromSeed` / `applySeedContent`; 409 when seed unusable
- [x] 1.2 Integration tests: happy path preserves presentation and applies bank experience; 409 with no usable seed; foreign/unauth unchanged

## 2. Web prompt after bank edits

- [x] 2.1 After a successful bank mutation, offer a confirm to refresh CV content (copy aligned with Reset); decline is a no-op; session dismiss after No to avoid confirm storms
- [x] 2.2 Tailor host: on agree, call existing `resetCvFromResume` for the open tailored CV and refresh workspace document state
- [x] 2.3 Profile Experience host: on agree, call the base reseed endpoint and refresh displayed base/profile CV state as applicable
- [x] 2.4 Do not prompt from the Skills tab or other non-bank profile edits

## 3. Guardrails

- [x] 3.1 Handler integration tests + web unit/component coverage for the offer/decline paths where the repo already tests similar UI
- [x] 3.2 `go vet -tags=integration ./...` for Go changes; run the web checks this package normally uses for touched files
