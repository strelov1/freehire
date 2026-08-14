## 1. Employment type & seniority — schema and backend wiring

- [x] 1.1 Add migration `job_submissions.employment_type text DEFAULT '' NOT NULL` and
      `.seniority text DEFAULT '' NOT NULL` (next migration number after the current
      highest in `migrations/`), mirroring `migrations/0031_submit_structured_facets.sql`.
- [x] 1.2 Add `EmploymentType`/`Seniority` to `createJobRequest`
      (`internal/handler/jobs_moderation.go`) and thread them through `toCreateInput()`.
- [x] 1.3 Add `EmploymentType`/`Seniority` to `moderation.CreateInput`, validate both in
      `CreateInput.structured()` via the existing `validEnum` helper against
      `vocab.EmploymentTypeValues`/`vocab.SeniorityValues`, and pass them into
      `jobderive.Input` inside `derive()`.
- [x] 1.4 Persist the submitted `employment_type`/`seniority` onto the `job_submissions`
      row and echo them back on the submission response, matching the existing
      `work_mode`/`regions` echo behavior.

## 2. URL prefill — parsing reuse, no persistence

- [x] 2.1 ~~Add a new non-persisting `Preview` method~~ — not needed:
      `linkimport.Importer.Resolve(ctx, raw string, known Board) (linksource.Resolved, bool, error)`
      (`internal/linkimport/linkimport.go:126`) already exists, already does not write
      (proven by `TestResolve_DoesNotWriteAnything`), and is the exact seam
      `internal/jdresolve` already uses for the same purpose. Design updated accordingly.
- [x] 2.2 Add `POST /api/v1/submissions/prefill` in `internal/handler/submissions.go`
      (`mw.key`, `mw.outboundFetch`), calling `im.Resolve(ctx, url, linkimport.Board{})`
      and projecting `title`/`company`/`location`/`description`/`work_mode`/
      `employment_type`/`seniority`/`source` (from `resolved.Job`/`resolved.Source`)
      into the response; `ok=false` returns `200` with empty fields.

## 3. Frontend contract

- [x] 3.1 Export `SENIORITY_OPTIONS`/`EMPLOYMENT_TYPE_OPTIONS` from `web/src/lib/facets.ts`
      (same pattern as the existing `WORK_MODE_OPTIONS` export).
- [x] 3.2 Regenerate/extend `web/src/lib/types.ts`: `SubmissionInput` gains
      `employment_type`/`seniority`; add a `PrefillResult` type for the new endpoint's
      response.
- [x] 3.3 Add `api.prefillSubmission(url)` to `web/src/lib/api.ts` calling
      `POST /api/v1/submissions/prefill`.

## 4. Submit form — new fields and prefill

- [x] 4.1 Add employment-type and seniority selects to `SubmitView.svelte`'s Details
      section, next to Work format, using the new facet options; wire them into the
      `submit()` payload.
- [x] 4.2 Add a "Fill in from this link" action next to the URL field: calls
      `api.prefillSubmission`, fills only fields the submitter has not already typed
      into, and no-ops silently when the response comes back empty.

## 5. Preview tab

- [x] 5.1 Build `web/src/lib/components/JobPreview.svelte`: presentational-only render
      of the draft `$state` (company/logo, title, salary via `formatSalary`, facets via
      `summaryFacets`, description via `JobDescription`) — no API calls, no
      apply/save/vote/report/discussion affordances.
- [x] 5.2 Add Details/Preview tabs to `SubmitView.svelte`; Preview renders
      `JobPreview` fed by the current form state.

## 6. Verification

- [x] 6.1 Manual browser check: fill the Details tab, confirm Preview matches a real
      `/jobs/[slug]` render for equivalent content.
- [x] 6.2 Manual browser check: paste a known recognized job URL, confirm prefill fills
      empty fields and does not clobber fields already typed; paste an unrecognized URL
      and confirm it degrades to a no-op, not an error toast.
- [x] 6.3 `go vet ./...` (pre-commit), `go vet -tags=integration ./...` (push-time),
      `go test ./...`, `gofmt -l .` clean before push.
