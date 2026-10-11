## 1. Backend: the retry endpoint

`Store.Status` (Meta.UploadedAt), `Store.Text` (re-derives text from the stored upload via
`extractText`'s existing `%PDF`-prefix check), and `deriveResumeArtifacts` already exist — no new
`Store` method needed. This task is a thin handler over the three.

- [x] 1.1 RED: in `internal/api/handler/resume_test.go`, write a failing test that
      `POST /me/resume/retry-extract` with no stored résumé (`Store.Status` returns
      `Present: false`) returns a client error and triggers no derivation.
- [x] 1.2 RED: write a failing test that, given a stored upload, the handler calls
      `deriveResumeArtifacts` with the text `Store.Text` returns and the `UploadedAt` from
      `Store.Status` — assert via the same fake/stub pattern `PutResume`'s existing tests use to
      observe `extractStructuredResume`'s effects (`MarkExtractFailed`/`SetStructured` calls, or
      an injected extractor double).
- [x] 1.3 GREEN: implement `RetryResumeExtract` in `internal/api/handler/resume.go`; wire the
      route (`api.Post("/me/resume/retry-extract", mw.cookie, h.RetryResumeExtract)`).

## 2. Frontend: surface the banner

- [x] 2.1 RED: find or create `ExperienceBankView`'s test file; write a failing test that a
      `GET /me/resume` response with `parse_status: 'failed'` renders a banner with its
      explanation text and a retry control, and that `parse_status: 'ok'` or no résumé present
      renders neither.
- [x] 2.2 GREEN: fetch `GET /me/resume` in `ExperienceBankView.svelte` (alongside the existing
      `load()`), add the banner — same `rounded-lg bg-warning/5` idiom as the existing
      unconfirmed-achievements banner — gated on `parse_status === 'failed'`.

## 3. Frontend: the retry action and its poll

- [x] 3.1 RED: write a failing test that activating Retry calls the new
      `retryResumeExtract()` API function (add it to `web/src/lib/api.ts`, `POST
      /me/resume/retry-extract`) and shows a busy/"retrying" state on the control.
- [x] 3.2 RED: write a failing test for the poll that follows: after the retry call resolves,
      the component polls `GET /me/resume` until `parse_status` is no longer `failed` (success
      → refresh the bank and hide the banner; a fresh `failed` → update the banner's detail
      text instead of polling forever) — mirror the onboarding wizard's distinction between
      `parse_status` (three-valued, terminates) and `structure_pending` (two-valued, would wait
      forever on a failed retry) rather than reusing its code.
- [x] 3.3 GREEN: implement the poll loop and the bank refresh on success.

## 4. Simplify, verify, review

- [x] 4.1 Run `simplify` over the diff.
- [x] 4.2 `go test ./internal/api/handler/...` — green; `go vet` clean.
- [x] 4.3 `pnpm check` / `pnpm test` in `web/` — unchanged baseline errors, new tests green.
- [x] 4.4 Request code review on the diff; fix Critical/Important findings.
