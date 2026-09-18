# Employer accounts conventions

## Scope

Claiming a company, verifying a work email against it, and — once active — publishing,
editing, and closing that company's own vacancies plus editing its curated profile. The
third manual-intake path, beside `internal/ingest/moderation` (staff-authored) and
`internal/ingest/submission` (public queue, mints through moderation). See
[openspec/changes/add-employer-company-accounts](../../../openspec/changes/add-employer-company-accounts)
for the full design record.

## Always true

- **One `Service`, two halves.** `account.go` (claim/verify/moderate/revoke) and `job.go`
  (create/edit/close a vacancy) share one type because every job-authoring action starts by
  calling the account half's own `ActiveAccount` guard — there is no caller outside this
  package that would ever want one half without the other, so a formal interface boundary
  between them would buy nothing.
- **`company_accounts` is not derived from `jobs`, like `company_slug_aliases`.**
  `companies` is rebuilt by `SyncCompaniesFromJobs`/`DeleteOrphanCompanies`; a claim has to
  outlive the row that motivated it, so it lives in its own table (migration 0174) with its
  own `UNIQUE(company_slug)` — which is also the whole race guard for two concurrent claims
  on the same company (one wins the insert, the other gets a mapped conflict).
- **`Account.CompanyName` is fixed at claim time and never re-typed.** Every vacancy this
  account creates or edits passes this exact string into derivation
  (`normalize.CompanySlug`), which is what keeps `company_slug` from drifting between two
  postings from the same employer. The company name in a create/update HTTP request is
  never read for this reason — see `job.go`'s `CreateVacancy`/`UpdateVacancy`.
- **The website-seed side effect belongs to moderator *approval*, never to the automatic
  domain-match path.** `ConfirmClaim` activates on a match but never writes — by
  construction, a match can only happen when the website was already known and equal to
  what it's "seeding." Only `ApproveClaim` (a human vouching for a pairing the domain check
  itself could not verify) seeds a blank website. The two were conflated in an earlier draft
  of this spec and caught by TDD, not by review — see the design doc's Decisions section if
  this distinction is ever tempting to "simplify" back together.
- **`CreateVacancy` guards a real gap in `UpsertManualJob`, not a hypothetical one.** That
  query's `ON CONFLICT` never checks `created_by`, so reusing `moderation.Service.Create`
  unguarded would let employer B silently take over employer A's vacancy by resubmitting the
  same URL under `source='employer'`. The pre-check (`JobRepository.Owner`, backed by the
  already-existing `GetJobBySourceExternalID`) is what closes it; re-creating under the same
  URL by the SAME owner is the (deliberate, no-separate-endpoint) reopen path.
- **`UpdateVacancy`/`CloseVacancy` cannot reuse moderation's own `Update`/`BySlug`.** Those
  are scoped to "any manually-authored job" (`created_by IS NOT NULL`), correct for trusted
  staff and wrong for a self-service actor. `UpdateEmployerJob`/`CloseEmployerJob` add
  `created_by = actor_id AND source = 'employer'` to the same shape.
- **Facet derivation never touches moderation's private `derive()`.** Both `job.go`'s
  `deriveEmployer` and moderation's own `derive()` are thin wrappers around the same shared,
  exported primitives (`job.New`/`jobderive.Input`) — see
  [docs/agents/company-identity.md](../../../docs/agents/company-identity.md)'s "every write
  path shares it" invariant. Exporting moderation's `derive()` for this one caller would be
  the wrong fix.
- **`ClaimMailer` is exported for a Go-mechanics reason, not an API-design one.** A nil
  `*emailnotify.AuthMailer` assigned through an *unexported* interface-typed parameter would
  still typecheck, but produces a non-nil interface wrapping a nil pointer — invisible to
  `Service.Claim`'s own `s.mailer == nil` guard, and a panic waiting to happen the first time
  `SendClaimVerificationCode` is actually invoked. Exporting the interface lets the wiring
  code (`internal/api/handler.Register`) declare a properly nil-typed variable instead. See
  the type's own doc comment before removing the export.
- **Curated profile writes are authoritative, never fill-gap.** Every other writer of
  `companies`' curated columns (`cmd/import-yc`, the Wikipedia backfill, ingest's
  adapter-supplied description) treats itself as one of several possibly-wrong sources and
  never overwrites another's value. A verified employer editing their own company is the
  subject speaking about itself — see `company-info`'s spec delta in this change, and
  `SetCompanyAccountProfile`'s own comment for exactly which columns and why
  `year_founded`/`employee_count`/`hq_country`/`subindustry` additionally needed a guard in
  `cmd/import-yc` that `tagline`/`company_info`/`industries` did not.
- **`company_types`/`company_sizes` are never employer-editable.** They are job-derived
  (`RefreshCompanyFacets`, from the company's own postings' enrichment) — an employer's own
  vacancies feed them automatically once published and enriched. `CompanyProfilePatch`
  deliberately has no field for either.

## Structure

- `employer.go` — `Account`, `Repository`, `CompanyProfilePatch`, the sentinel errors.
- `account.go` — `Service`'s claim/verify/moderate/revoke half, `codeIssuer`/`ClaimMailer`
  ports.
- `job.go` — `Service`'s create/edit/close half, `JobRepository`/`Minter` ports,
  `VacancyInput`/`VacancyPatch`.
- `webmail.go` — the public-webmail-domain blocklist (`gmail.com` etc.) and the email-domain
  extraction helper both halves' domain checks share.
- `repository.go` — `QueriesRepository`, satisfying both `Repository` and `JobRepository`
  (one adapter, matching `Service`'s own single-type shape) over sqlc.

## Gotchas

- **A user_email_codes purpose needs its own migration, not just a Go constant.**
  `user_email_codes.purpose` carries a CHECK constraint (migration 0041) that predates this
  package; adding `accounts.PurposeVerifyWorkEmail` in Go alone 500s every `Claim` call until
  the constraint is widened (migration 0176 is the template — same DROP/ADD shape as the
  `jobs.closed_reason` widenings, sized down since this table is small and short-lived).
- **`normalize.CompanySlug` strips legal-form tokens, including bare `"Co"`.** A company
  named e.g. `"Widgets Co"` slugs identically to `"Widgets"` — this is correct (see
  [docs/agents/company-identity.md](../../../docs/agents/company-identity.md)) and not a bug
  in the claim flow, but it is easy to write a wrong test fixture expecting otherwise.
- **`GetCompany`'s `company_info` unmarshal must use `map[string]any`, never
  `map[string]string`.** The JSONB carries non-string values under keys this package does
  not touch (e.g. a subsidiaries array); unmarshalling into a string-valued map fails the
  WHOLE object the moment any other key isn't a string, which would silently read
  `description`/`website` as absent too. See `internal/api/handler/employer.go`'s
  `companyInfoField`.
