## Why

Every posting in the catalogue sends the candidate somewhere else to apply, including the
ones a verified employer authored here (`internal/ingest/employer` requires an external URL
for each vacancy). An aggregator whose own employers cannot take applications on it has
nothing to offer them that their careers page lacks, and nothing a candidate can do only
here. Letting a verified employer accept applications on freehire is what makes a vacancy
exclusive to us, and it builds on what already exists: claimed accounts with a confirmed
work email, the candidate's stored CV, the screening answer bank, and the captured
apply-form shape.

## What Changes

- A verified employer chooses, per vacancy, between an external apply link (today's
  behaviour) and **applying on freehire**. The second mints a vacancy with no external URL.
- Such a vacancy carries the employer's own screening questions — free text, yes/no, or a
  choice from a list, each optionally required — stored as an `apply_forms` row with
  `provider = 'freehire'`, the same `applyform.Form` shape captured ATS forms use.
- A signed-in candidate with a verified email applies from the job page: picks one of their
  CVs, answers the questions (prefilled from their answer bank), adds an
  optional note, and sends. The application is recorded once per `(job, candidate)`.
- The application is delivered by email to the employer account's confirmed work address,
  the CV attached as a PDF and `Reply-To` set to the candidate. It is stamped sent only after
  delivery succeeds; a failed delivery is retried by sending again, never duplicated.
- The application marks the job applied in the candidate's tracking, and new answers are
  saved to the candidate's answer bank.
- A per-candidate daily cap on applications, configurable by env.
- The job page shows an "Only on freehire" badge and an Apply button for these vacancies;
  their `JobPosting` markup declares `directApply: true`.
- The privacy page states that a CV reaches an employer only when the candidate sends it.

Not in this change (the seam is left open): an applicant inbox for the employer, application
statuses, payment for posting, auto-apply onto these forms.

## Capabilities

### New Capabilities
- `employer-direct-apply`: an employer-authored vacancy that accepts applications on
  freehire — its question form, the candidate's application, delivery to the employer,
  the daily cap, and how the job page and its structured data present it.

### Modified Capabilities
- `ojcp-provider`: an on-site vacancy reports a `platform_native` apply path instead of an
  `external_redirect` with no URL.

## Impact

- New migration: `direct_applications`.
- `internal/ingest/moderation`: `CreateInput` accepts a caller-minted external id in place of
  a URL.
- `internal/ingest/employer`: vacancy create/edit accept an apply mode and questions.
- New `internal/ingest/directapply`: the application use case, with a mailer port wired in
  `internal/api/handler.Register` (the `ClaimMailer` pattern); added to the layering table.
- `internal/job/jobview`: an `apply_mode` field on the wire.
- `internal/platform/db/queries`: new queries, `make sqlc`.
- `internal/api/handler`: apply endpoint, form read endpoint, employer create/edit fields.
- `internal/engage/emailnotify`: an application email with attachment and Reply-To.
- `web/`: question editor in `/my/employer`, apply form on the job page, badge,
  `directApply` in JSON-LD, privacy text.
- `web/static/openapi.yaml`: the new endpoints.
