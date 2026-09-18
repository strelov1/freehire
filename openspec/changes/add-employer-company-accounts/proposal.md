## Why

Every vacancy in the catalogue today arrives through a crawler adapter, a moderator's hand
entry, or the public submission queue — an employer has no way to represent itself, correct
its own company profile, or publish a posting directly. This blocks the site's stated
direction of letting employers manage their own presence and, eventually, publish postings
exclusive to freehire with in-house applications; this change lays the first, self-contained
piece of that path: a verified employer account that can edit its company's curated profile
and publish/edit/close its own vacancies, with nothing else about the catalogue's existing
write paths touched.

## What Changes

- New `company_accounts` table linking a `users` row to one `company_slug`, with a
  `pending → active → revoked` status lifecycle. Claiming reserves the slug immediately
  (`UNIQUE(company_slug)`), which also resolves a double-claim race.
- New claim/verification flow: resolve or mint the company slug, capture a work email,
  reject public webmail domains, verify by mailed code, and auto-activate when the email's
  domain matches the company's already-known website — otherwise the claim sits in a small
  moderator review queue instead of being refused outright.
- New `internal/ingest/employer` package: a verified employer may create, edit, and close
  vacancies attributed to their own claimed company only. Creation reuses the existing
  moderator-authored-job write path (`source = 'employer'`); edit and close are new,
  actor-scoped write paths — the existing moderator edit path is scoped only to "any
  manually-authored job" and is not safe to reuse for a self-service actor.
- New `closed_reason = 'employer_closed'` value, following the existing incremental
  CHECK-constraint migration pattern.
- A verified employer becomes a new authoritative writer of `companies`' curated fields
  (`tagline`, `company_info.description`, `company_info.website`, `industries`,
  `year_founded`, `employee_count`, `hq_country`, `subindustry`) for their own company —
  `cmd/import-yc` gains a guard so it no longer silently overwrites employer-asserted
  `year_founded`/`employee_count`/`hq_country`/`subindustry`.
- New `POST /api/v1/employer/*` routes (claim flow, company profile, job CRUD), a small
  moderator-facing review queue for pending claims, and new SvelteKit pages for the claim
  flow and a company dashboard.
- Explicitly **not** in this change: claiming/editing a company's existing crawler-ingested
  postings, multi-seat/team access to one company account, any paid gate, and anything about
  exclusive postings or in-house application intake (deferred future work this change's data
  model — `company_accounts` plus `jobs.source = 'employer'` — deliberately does not block).

## Capabilities

### New Capabilities

- `employer-account`: claiming a company, work-email domain verification, the
  pending/active/revoked account lifecycle, and the moderator review queue for a claim that
  cannot auto-verify.
- `employer-job-authoring`: a verified employer creating, editing, and closing vacancies
  scoped to their own claimed company, including the guard against another employer taking
  over a posting via a colliding URL.

### Modified Capabilities

- `company-info`: a verified employer becomes an additional authoritative writer of a
  company's curated info fields, and `cmd/import-yc`'s existing writer must not clobber
  employer-asserted `year_founded`/`employee_count`/`hq_country`/`subindustry`.

## Impact

- **Database**: new migration for `company_accounts`; new migration adding
  `'employer_closed'` to `jobs_closed_reason_check`.
- **Go**: new `internal/ingest/employer` package; small additions to
  `internal/identity/accounts` (generic `IssueCode`/`ConfirmCode`, a new `CodeMailer` method,
  a new purpose constant); a guard added to `cmd/import-yc`'s upsert; new sqlc queries
  (actor-scoped update/close, a public-webmail-domain check has no DB component).
- **API**: new `internal/api/handler` routes under `/api/v1/employer/*`; a small addition to
  the existing moderator admin surface for the claim review queue.
- **Web**: new SvelteKit pages for the claim flow and a company dashboard (job list, profile
  edit form).
- **No changes** to `internal/ingest/moderation`, `internal/ingest/submission`, the ingest
  pipeline, or search/indexing beyond the existing `search_outbox` enqueue every manual write
  path already uses.
