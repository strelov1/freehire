# Direct CV extraction implementation plan

**Goal:** Remove the local PII detector and extract the complete structured CV in one schema-constrained LLM call, with the existing conditional date correction.

**Reason:** The audit observed approximately 7,300 successful detector log entries and ten API-side detector timeouts over seven days. Four short synthetic requests took 5–14 seconds each. Historical failures included Unicode offsets, partial names, and employment dates treated as phone numbers. These measurements do not establish a model error rate or justify claiming every request fails.

**Design:** The extraction model receives bounded original CV text and returns contacts alongside experience and education. Missing facts remain empty. Keep sanitation, date validation, caller attribution, upload-stamp persistence guards, and downstream contact-free/public projections. Remove the detector service, Go package, configuration, wiring and service-specific CI. The user explicitly requested this removal and verification; the raw CV will now reach the configured LLM gateway.

- [x] Add regression tests for direct contact extraction without a detector, multilingual source preservation, unchanged employment dates and missing contacts; observe failures before implementation.
- [x] Simplify Extractor to depend on its LLM client only; include contacts in its schema and prompt, regenerate the schema golden, and update callers and existing tests.
- [x] Remove orphaned detector code/configuration/CI, update current architecture/specification documentation, and record the retirement rationale under `.out-of-scope/`.
- [x] Run focused tests, Go build/vet/unit tests, integration-tag compilation and appropriate integration tests. Exercise the CV upload/parse flow in an isolated verify-freehire instance and run live model checks if credentials are available without exposing them.
- [x] Review the final diff and fix any confirmed regressions. Report the evidence and any checks that could not run; do not claim deployment.


## Verification evidence

- Focused unit tests: `resumeextract`, `config`, `layering`, and API handlers pass.
- Live model: English, Cyrillic and Portuguese contacts, absent contacts, and five dated
  employments pass. Short synthetic extractions took approximately 3–5 seconds each.
- Playwright public API journey: guest onboarding, registration, text CV storage, exact
  name/email and employment months, retry from stored bytes, and multipart PDF upload pass.
  The PDF's structured model result includes Go and PostgreSQL. The existing deterministic
  dictionary contract for that fixture is Docker, Kubernetes and PostgreSQL.
- Database evidence: extraction status `ok`, derive stamp equals the latest upload stamp,
  and expected model skills are persisted. The isolated database, services, S3 test bucket
  and temporary gateway credential file were removed after verification.
- Independent review found a contact URL copied into achievement prose; a failing regression
  test reproduced it. Deterministic cleanup of already-extracted contacts now rejects those
  semantic strings before experience import, while preserving owner contacts. Unicode name
  boundaries avoid deleting unrelated words, and values are trimmed before comparison.
  The regression and follow-up review pass. This adds no detector or model call.
- Markdown links, file-size checks, formatting and whitespace checks pass.

### Broader verification limitations

The first broad tagged API suite on the memory-constrained laptop timed out after ten
minutes. It reported failures in:

- `TestResetPassword_ConcurrentRequestsConsumeTheCodeOnce`
- `TestChangePasswordKeepsTheCallerSignedInAndRevokesOthers`
- `TestJobAutoApplyRetryAndCancelRoutes`
- `TestPostAutoApplyTailor_ASpentBudgetStillRecordsTheTailoredCV`
- `TestRunAgentAutofillAnswersEachFailureAsWhatItIs`
- `TestEmployerEndToEnd`
- `TestPutProfile_ValidationErrors`

The log contains request/deadline timeouts, including in unrelated account and employer
flows; this run is not reported as passing. The targeted tagged `Resume|OnboardingCV` API
suite subsequently passes in 11.3 seconds. Full `go build ./...`, `go vet ./...`,
`go vet -tags=integration ./...` and `go test ./...` all pass in a disposable Linux checkout
of the same base plus this patch. The staged Go diff SHA-256 was identical on both machines.
The disposable Linux checkout was removed after verification.

A browser navigation journey hit the development server's 60-second Lucide module-import
timeout. The full extraction journey was therefore verified through public APIs with
Playwright; successful browser rendering is not claimed. No production deployment or
live service shutdown was performed.

## Final shipping scope

Remove the legacy placeholder cleanup and its obsolete regression test too. Original
uploads were never rewritten with placeholders; no concrete compatibility need was
established. Known-contact cleanup remains because professional facts reach the
experience bank and agent-facing projections.
