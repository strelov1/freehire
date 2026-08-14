# Submit page redesign: preview tab, structured fields, URL prefill

**Date:** 2026-08-13
**Status:** approved, ready for planning

## Problem

`/submit` (`web/src/lib/components/SubmitView.svelte`) is one long form: paste a URL,
fill in title/company/location/facets/salary/description by hand, submit blind — the
submitter never sees how the posting will actually read on the site before it goes to
moderation. Two structured facets the schema already carries end-to-end
(`jobs.employment_type`, `jobs.seniority`) are missing from the form entirely, so every
submitted vacancy loses them to whatever the title/description dictionaries can guess.
And a submitter who has the posting's own URL still retypes everything the source page
already states, even though freehire already has a general-purpose "parse a job URL"
engine — it just isn't reachable from this form.

## Solution

Three additive, independently shippable changes to the same page. None touch pricing or
gate the form behind anything new; the endpoint stays `mw.key`-authenticated and
moderator-reviewed exactly as today.

### 1. A Preview tab

Two tabs above the form: **Details** (today's fields plus the two new ones) and
**Preview** — a live render of how the vacancy will look once approved.

`JobView.svelte` is not reusable for this: it is wired to a persisted job (`job.public_slug`
drives view/apply recording, discussion thread count, save/vote, the report dialog — all
API calls that assume a row exists). A draft has none of that. Instead, a new
`JobPreview.svelte` renders only the presentational slice, reusing existing pieces rather
than re-deriving them:

- `EntityLogo` + `companyLogoUrl` for the header
- `formatSalary` over the form's manual salary fields
- `summaryFacets` over the form's structured facets, built into a minimal `Job`-shaped
  object from current `$state` (no network call — it updates on every keystroke)
- `JobDescription` for the sanitized-HTML render of the markdown editor's output

No apply CTA, no save/vote/report/discussion — those all presuppose a stored job. The tab
switch is instant since nothing is fetched.

### 2. Employment type and seniority as submitter-stated fields

Both already exist as columns (`jobs.employment_type`, `jobs.seniority`,
`migrations/0001_init.sql:254,259`) and as override slots on the derivation input
(`jobderive.Input.EmploymentType`/`.Seniority`, `internal/jobderive/jobderive.go:51,53`,
precedence "structured signal → title dictionary → description",
`jobderive.go:135–158`) — but `moderation.CreateInput` never populates them
(`internal/moderation/moderation.go:239–262` builds `jobderive.Input` without either).
This wires the same path `work_mode`/`regions` already use:

- `createJobRequest` (`internal/handler/jobs_moderation.go:17`) gains `EmploymentType`,
  `Seniority` — same shape as `WorkMode`.
- `CreateInput.structured()` (`moderation.go:220`) validates them with the existing
  `validEnum` helper against `vocab.EmploymentTypeValues`/`vocab.SeniorityValues`
  (`internal/vocab/vocab.go:31,34`) — an out-of-vocabulary value is dropped, degrading to
  dictionary derivation exactly like an unknown `work_mode` does today.
- `derive()` (`moderation.go:239`) passes both into `jobderive.Input`.
- A migration (`0094` — `0093` was taken by a parallel PR by the time this shipped) adds
  `employment_type text DEFAULT '' NOT NULL` and
  `seniority text DEFAULT '' NOT NULL` to `job_submissions`, mirroring
  `migrations/0031_submit_structured_facets.sql`'s pattern — so "My submissions" can echo
  back what the submitter actually stated, not just what got derived.

Frontend: `web/src/lib/facets.ts` already builds `SENIORITY`/`EMPLOYMENT` facet-option
arrays internally (`facets.ts:373,375`) for the filter bar; it exports `WORK_MODE_OPTIONS`
etc. but not these two. Adding `export const SENIORITY_OPTIONS = SENIORITY` /
`EMPLOYMENT_TYPE_OPTIONS = EMPLOYMENT` (same one-line pattern as line 390) lets the form
use the identical dictionary and labels the filter bar shows, so a submitter picks from
the same vocabulary a browsing candidate filters on. Two new pill/select groups sit next
to "Work format" in the Details tab.

### 3. Prefill from a job URL

The form's URL field gains a "Fill in from this link" action. It does not touch the
existing paste-a-link contribution flow (`POST /jobs/resolve`, `internal/handler/contributions.go:45`)
at all — that flow imports directly into the catalog and rewards credits, which is the
wrong outcome for a submission that is supposed to reach a moderator queue. What it
reuses is the *parsing engine* underneath both flows: `internal/linksource`, the same
registry `linkimport.Importer` already holds (host-scoped adapters for greenhouse,
ashby, lever, workable, habrcareer, remoteyeah, geekjob, bairesdev → board coverage over
every ATS `internal/atsboard` recognizes → a generic `JobPosting` schema.org fallback for
an arbitrary careers page, `internal/linksource/registry.go:49–75`). This is materially
broader than hand-writing fetchers for a handful of ATS platforms, and it is already
built, tested, and running in production behind `/jobs/resolve`.

No new method was needed: `linkimport.Importer` already exposes
`Resolve(ctx, raw string, known Board) (linksource.Resolved, bool, error)`
(`internal/linkimport/linkimport.go:126`) — `Import` is defined as `Resolve` + `Write`,
and `Resolve` alone is the exact seam `internal/jdresolve` already uses to branch on
source before deciding how to persist, proven not to write by its own
`TestResolve_DoesNotWriteAnything`. The prefill handler calls
`im.Resolve(ctx, url, linkimport.Board{})` directly and reads the parsed `sources.Job`
(title/company/location/description, and — when the platform states them structurally —
`WorkMode`/`Seniority`/`EmploymentType`/`Skills`, `internal/sources/source.go:33–57`) off
`resolved.Job`; `ok=false` when nothing matched or the page is not a single posting.

New handler: `POST /submissions/prefill { url }`, `mw.key` + `mw.outboundFetch` (the same
throttle `/jobs/resolve` already sits behind — this endpoint makes the same class of
outbound request). Response mirrors `SubmissionInput`'s shape minus the arrays freehire's
own dictionaries derive better than any source page states them (skills, cities): `title`,
`company`, `location`, `description`, `work_mode`, `seniority`, `employment_type`,
`source` (the resolved platform key, prefilling the existing "Source" field). A miss
(`ok=false`) returns 200 with an empty body's worth of nulls, not an error — the
submitter just keeps typing, same as if they'd never clicked the button.

Frontend: fills the corresponding `$state` fields on success. Fields already carrying
user input are not overwritten — the submitter typing a title before pasting the link
must not have it clobbered.

## Components

| File | Role |
|---|---|
| `web/src/lib/components/JobPreview.svelte` | new — presentational-only render of the draft form state |
| `web/src/lib/components/SubmitView.svelte` | edit — tabs (Details/Preview), two new fields, prefill button |
| `web/src/lib/facets.ts` | edit — export `SENIORITY_OPTIONS`, `EMPLOYMENT_TYPE_OPTIONS` |
| `web/src/lib/types.ts` | edit — `SubmissionInput` gains `employment_type`/`seniority`; new `PrefillResult` type |
| `web/src/lib/api.ts` | edit — `prefillSubmission(url)` calling `POST /submissions/prefill` |
| `internal/handler/jobs_moderation.go` | edit — `createJobRequest`/`toCreateInput` gain the two fields |
| `internal/moderation/moderation.go` | edit — `CreateInput`, `structured()`, `derive()` wire them into `jobderive.Input` |
| `internal/linkimport/linkimport.go` | edit — new `Preview` method, no persistence |
| `internal/handler/submissions.go` | edit — new `POST /submissions/prefill` route (`api.Post("/submissions", ...)` sits at `submissions.go:33`) |
| `migrations/0093_submission_employment_seniority.sql` | new — `job_submissions.employment_type`/`.seniority` columns |
| `cmd/gen-contracts` output (`web/src/lib/types.ts` generated section) | regenerate after the Go wire types change |

## Testing

- Go: unit tests for `CreateInput.structured()` validating employment_type/seniority
  against the vocab (unknown value dropped, mirroring the existing `work_mode` test);
  a `linkimport.Preview` test against a canned page confirming it returns a parsed job
  and performs no DB write (table stays empty).
- Integration-tagged: `POST /submissions/prefill` against a stubbed transport — matched
  page, unmatched URL (`ok=false`, 200), and the existing auth/throttle middleware applies.
- Frontend: no component test runner in this project (`hire-web-no-test-runner`) —
  verify manually in a live browser: fill the Details tab, switch to Preview and confirm
  it matches a real `/jobs/[slug]` render for the same content; paste a known greenhouse
  URL and confirm prefill fills fields without clobbering anything already typed; paste
  an unrecognized URL and confirm it degrades to a no-op, not an error toast.
