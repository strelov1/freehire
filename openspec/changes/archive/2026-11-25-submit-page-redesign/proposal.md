## Why

`/submit` is a single long form: a submitter pastes a URL and retypes everything by
hand, with no sense of how the vacancy will actually read once approved. Two facets the
schema already carries end-to-end (`employment_type`, `seniority`) are absent from the
form, so every submission loses them to dictionary guesswork. And freehire already runs
a general-purpose "parse a job URL" engine (the same one behind the paste-a-link
contribution flow) that the submit form never calls, so a submitter retypes what the
source page already states.

## What Changes

- Add a Preview tab to `/submit` that renders the in-progress draft the way it will look
  once approved, using a new presentational-only component (no job row exists yet, so it
  makes no API calls).
- Add `employment_type` and `seniority` as submitter-stated structured facets, wired
  through the existing override path (`jobderive.Input`) the way `work_mode`/`regions`
  already are.
- Add a "fill in from this link" action on the URL field: a new, non-persisting
  `POST /submissions/prefill` endpoint that reuses the existing `internal/linksource`
  parsing registry (the same one `POST /jobs/resolve` uses to import) to parse
  title/company/location/description/facets from the pasted URL, without writing
  anything or awarding credits. A miss degrades silently to manual entry.

## Capabilities

### New Capabilities

(none — this extends the existing submission surface)

### Modified Capabilities

- `job-submission`: the `/submit` form gains employment-type/seniority inputs and a
  Preview tab; the submission API gains `employment_type`/`seniority` fields on
  `POST /api/v1/submissions` (stored and echoed back, same contract as the existing
  structured facets); a new `POST /api/v1/submissions/prefill` endpoint parses a job URL
  into draft field values without persisting anything.

## Impact

- Backend: `internal/handler/jobs_moderation.go`, `internal/handler/submissions.go`,
  `internal/moderation/moderation.go`, `internal/linkimport/linkimport.go`, a new
  migration adding two columns to `job_submissions`.
- Frontend: `web/src/lib/components/SubmitView.svelte`, a new
  `web/src/lib/components/JobPreview.svelte`, `web/src/lib/facets.ts`,
  `web/src/lib/types.ts`, `web/src/lib/api.ts`.
- No changes to `POST /jobs/resolve` or the credit-reward contribution flow — the new
  endpoint only borrows the parsing step, never the import/reward path.
- Full design in `docs/superpowers/specs/2026-08-13-submit-page-redesign-design.md`.
