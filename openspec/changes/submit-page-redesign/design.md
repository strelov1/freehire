Full file:line references and code snippets live in
`docs/superpowers/specs/2026-08-13-submit-page-redesign-design.md`; this document states
the decisions and their rationale for OpenSpec tracking.

## Context

`/submit` (`web/src/lib/components/SubmitView.svelte`) posts to
`POST /api/v1/submissions` (`internal/handler/submissions.go:33`), which shares its
validation/derivation path with the moderator-create endpoint via
`moderation.CreateInput` (`internal/moderation/moderation.go`). Two columns the schema
already has — `jobs.employment_type`, `jobs.seniority` — and their corresponding
override slots on `jobderive.Input` are wired for moderator-authored jobs but never
populated by the submission path. Separately, `internal/linksource` already parses an
arbitrary job-detail URL into a `sources.Job` (title/company/location/description/
structured facets) for the paste-a-link contribution flow (`POST /jobs/resolve`,
`internal/handler/contributions.go:45`) — that flow imports directly into the catalog
and pays a credit reward, which is the wrong outcome for content that must still clear
moderation.

## Goals / Non-Goals

**Goals:**
- Let a submitter see, before submitting, a render of the vacancy close to how it will
  look once approved.
- Let a submitter state employment type and seniority explicitly, on the same override
  path `work_mode`/`regions` already use.
- Let a submitter prefill the form from a job URL using freehire's existing URL-parsing
  coverage, without duplicating that parsing logic.

**Non-Goals:**
- No pricing/paid tier on `/submit` — it stays free and moderator-reviewed, unchanged.
- No change to `POST /jobs/resolve` or the credit-reward contribution flow — prefill is
  strictly read-only and shares only the parsing registry, not the import/reward path.
- No new submitter-facing fields beyond employment type and seniority (application
  deadline, company website/logo, employer screening questions) — none of these exist
  anywhere in the schema today and each would need its own migration/table; out of scope
  for this change.

## Decisions

**Preview via a new presentational component, not a reused `JobView`.** `JobView.svelte`
is wired to a persisted job: `job.public_slug` drives view/apply recording, discussion
thread count, save/vote, and the report dialog, all of which are API calls against a row
that does not exist for a draft. `JobPreview.svelte` instead reuses only the
presentational pieces (`EntityLogo`, `companyLogoUrl`, `formatSalary`, `summaryFacets`,
`JobDescription`) against the form's live `$state`, with no apply/save/vote/report/
discussion affordances and no network call — the tab switch is instant.

**Employment type / seniority follow the existing override precedence exactly**
(structured signal → title dictionary → description phrase, `jobderive.go:135-158`), so
adding them to the submission path is additive: extend `createJobRequest` →
`CreateInput.structured()` → `jobderive.Input`, validated by the existing `validEnum`
helper against `vocab.EmploymentTypeValues`/`vocab.SeniorityValues`. An
out-of-vocabulary value is dropped rather than rejected, matching how an unknown
`work_mode` already degrades to derivation.

**Prefill reuses `internal/linksource` via a new non-persisting `Preview` method on
`linkimport.Importer`**, rather than either (a) hand-writing new per-ATS fetchers, which
would cover only a handful of platforms `internal/linksource` and its board-coverage
adapter already exceed, or (b) calling `POST /jobs/resolve` directly, which commits an
import and a credit reward — the wrong side effect for a moderated submission. `Preview`
runs the identical `linksource.Find(...).Resolve(...)` step `Import` does, skipping the
dedup check, enrichment enqueue, and search push. A miss (`ok=false`) is a normal
outcome, not an error: the endpoint returns 200 with empty fields and the submitter
keeps typing.

**The prefill endpoint sits behind the same `mw.outboundFetch` throttle `/jobs/resolve`
uses** — it makes the same class of server-initiated outbound request against an
arbitrary user-supplied URL, so it inherits the existing abuse-rate guard rather than
introducing a new one.

## Risks / Trade-offs

- **Prefill can be slow or fail** (parsing an arbitrary third-party page) → the button is
  explicit and optional, never blocks manual entry, and already-typed fields are never
  overwritten by a late-arriving prefill response.
- **`JobPreview` can drift from `JobView`'s real rendering** since it is a second
  implementation of the same presentational slice → both draw from the same shared
  helpers (`summaryFacets`, `formatSalary`, `JobDescription`, `companyLogoUrl`) rather
  than duplicating their logic, so a facet-rendering change made in one place is visible
  in both.
- **Adding columns to `job_submissions`** is a plain additive migration (`ADD COLUMN ...
  DEFAULT '' NOT NULL`), the same shape as the precedent in
  `migrations/0031_submit_structured_facets.sql` — no backfill, no lock risk beyond a
  normal `ALTER TABLE ADD COLUMN` with a constant default.

## Migration Plan

1. Ship the backend change (migration + handler/moderation wiring +
   `linkimport.Preview`) first; it is additive and inert until the frontend calls it.
2. Ship the frontend change (new fields, Preview tab, prefill button) in the same or a
   follow-up deploy — no ordering hazard either way since the new endpoint/fields are
   purely additive to an existing contract.
3. No feature flag: `/submit` already requires sign-in, and every change here is
   additive to that same gated surface.

Rollback: revert the frontend deploy; the backend fields/endpoint are inert if unused
and need not be rolled back separately.

## Open Questions

None outstanding — scope, approach, and non-goals were confirmed during brainstorming
(see the linked design doc).
