## 1. Database schema

- [x] 1.1 Migration: create `company_accounts` (`user_id` PK/FK → `users(id)` cascade,
      `company_slug` NOT NULL UNIQUE, `company_name` NOT NULL, `work_email` NOT NULL,
      `status` NOT NULL DEFAULT `'pending'` CHECK IN (`pending`,`active`,`revoked`),
      `verified_at` nullable, `created_at`/`updated_at`)
- [x] 1.2 Migration: add `'employer_closed'` to `jobs_closed_reason_check`
      (`DROP CONSTRAINT IF EXISTS` → `ADD CONSTRAINT ... NOT VALID` → `VALIDATE CONSTRAINT`,
      matching migrations/0147, 0159, 0165)

## 2. `internal/identity/accounts`: generic code primitives

- [x] 2.1 Add `PurposeVerifyWorkEmail` constant next to `PurposeVerifyEmail`/`PurposeResetPassword`
- [x] 2.2 Add `Service.IssueCode(ctx, userID, purpose, email, send) error` and
      `Service.ConfirmCode(ctx, userID, purpose, code) error`, factored out of the existing
      private `issueCode`/`consumeCodeTx` (no side effect on confirm — caller decides what
      "verified" means). **Refined during implementation**: `IssueCode` takes the delivery as
      an explicit `send func(ctx, email, code) error` parameter rather than routing through
      `CodeMailer` — this needs no change to that interface at all (see 2.4). `ConfirmCode` is
      a new sibling of `ConfirmVerification`, not something `ConfirmVerification` itself calls:
      `ConfirmVerification` bundles the code-consume and `MarkEmailVerified` in one transaction
      (see `docs`/AGENTS.md's "a reset is spend-the-code-plus-write-the-value or nothing"), and
      routing it through a `ConfirmCode` that commits on its own would split that atomicity.
- [x] 2.3 Re-point `IssueVerificationCode` to call the new generic `IssueCode` (existing tests
      stay green, unchanged behavior). `ConfirmVerification` is intentionally left as its own
      standalone transaction, for the reason above — not "re-pointed."
- [x] 2.4 ~~Add a `CodeMailer` method for the employer-claim verification email~~ — superseded
      by the 2.2 refinement: `internal/ingest/employer` defines its own small mailer interface
      (`claimMailer`) and passes its method straight to `accounts.Service.IssueCode`'s `send`
      parameter (see 3.4). No change to `accounts.CodeMailer` needed.

## 3. `company_accounts` domain: claim and verification

- [x] 3.1 sqlc queries: insert pending `company_accounts` row, get by `user_id`, list
      `status='pending'`, activate, revoke, delete (reject), plus `SeedCompanyAccountWebsite`
      and reuse of the existing `GetCompanySlugAlias`/`GetCompany` — `internal/ingest/employer`'s
      `Repository`/`QueriesRepository`, unit-tested against fakes and integration-tested
      against real Postgres (unique-constraint mapping, the seed guard, alias resolution)
- [x] 3.2 Small public-webmail-domain rejection list + a check function (`webmail.go`)
- [x] 3.3 `Service.Claim`: resolve slug via `company_slug_aliases`/`normalize.CompanySlug`;
      reject a public-webmail work email; insert the pending row (its unique-constraint
      conflicts map to `ErrAlreadyHasAccount`/`ErrCompanyAlreadyClaimed`); issue a code via
      `accounts.Service.IssueCode`. **Merged 3.3+3.4 from the original breakdown** — one
      request carrying company name and work email together, not two, which avoids a
      nullable `work_email` column for no behavioral loss (see design.md).
- [x] 3.5 `Service.ConfirmClaim`: confirm via `accounts.Service.ConfirmCode`; compare the
      email's domain to `companies.company_info->>'website'` when known; activate on match,
      else leave `pending`. **Corrected during implementation**: does NOT seed
      `company_info.website` — see 3.6, the spec as originally written was self-contradictory
      (auto-activate-on-match can only fire when the website is already known and matching,
      so "seed if blank" could never apply there).
- [x] 3.6 Moderator review service: list pending claims, reject (delete, freeing the slug),
      and **approve** — which both activates AND seeds `company_info.website` from the
      claim's confirmed work-email domain when it was blank, since a moderator approving a
      claim the domain check could not verify is exactly the human vouching that gap needed
      (see the corrected `employer-account` spec).
- [x] 3.7 Admin revoke action (sets `status='revoked'`, row and slug stay reserved)
- [x] 3.8 Ownership guard helper: resolve the active `company_accounts` row for an actor,
      used by every employer-facing capability (profile edit, job create/update/close)

## 4. `internal/ingest/employer`: job authoring

- [x] 4.1 Create: `Minter`-pattern wrapper around `moderation.Service.Create`
      (`source='employer'`, `Company` always the claimed `company_name`); pre-check via
      `GetJobBySourceExternalID(ctx, "employer", url)` refusing a different owner's URL
      (`ErrURLTaken`)
- [x] 4.2 Regression test: re-`Create` with the same URL by its own owner updates/reopens the
      existing vacancy rather than duplicating it — unit (fakes) and integration (real
      Postgres, confirms `closed_at` actually clears)
- [x] 4.3 Regression test: a different employer cannot take over a vacancy via a colliding
      URL — unit and integration (confirms the real row is untouched)
- [x] 4.4 New sqlc query `UpdateEmployerJob` (`WHERE public_slug = $slug AND created_by =
      $actorID AND source = 'employer'`) — `job.Fields.UpdateEmployerParams`, mirroring
      `UpdateManualParams`, added alongside it in `internal/job/job/job.go`
- [x] 4.5 Update service: re-derives facets via `job.New(job.Draft{Input: jobderive.Input{...}})`
      directly (not via `moderation`'s private `derive()`); URL/company stay immutable
- [x] 4.6 New sqlc query `CloseEmployerJob` (`closed_at`, `closed_reason='employer_closed'`,
      feeds `search_delete_outbox` — mirrors the existing `CloseJobByID`)
- [x] 4.7 Regression test: employer B cannot update or close employer A's vacancy (refused as
      `ErrJobNotFound`) — unit and integration

## 5. `cmd/import-yc` guard

- [x] 5.1 Guard added entirely in SQL (`UpsertYCCompany`'s `ON CONFLICT` clause, CASE-guarded
      on `EXISTS (... company_accounts ... status='active')`) — **no Go change needed** in
      `cmd/import-yc` itself, since its call site already just passes the params through
- [x] 5.2 Regression tests in `internal/platform/db/company_yc_integration_test.go` (the
      existing home of `UpsertYCCompany`'s own integration tests): an employer-asserted
      `year_founded`/`employee_count`/`hq_country`/`subindustry` all survive a re-import,
      every other YC-owned facet (`yc_batch` etc.) still applies normally, and a company
      with no employer account is unaffected (still overwritten exactly as before)

## 6. API handlers

- [x] 6.1 `POST /api/v1/employer/claim` (company_name + work_email together — see 3.3's
      merge note), `POST /api/v1/employer/claim/confirm`
- [x] 6.2 `GET`/`PATCH /api/v1/employer/company` (curated profile fields only — never
      `company_types`/`company_sizes`, which stay job-derived). New service-layer support
      added alongside the handler: `employer.CompanyProfilePatch` +
      `Service.UpdateCompanyProfile`, a new `SetCompanyAccountProfile` sqlc query
      (nil-means-unchanged via `sqlc.narg`/COALESCE), reusing the existing
      `SetCompanyIndustries` for the industries field.
- [x] 6.3 `POST /api/v1/employer/jobs`, `PATCH /api/v1/employer/jobs/:slug`,
      `POST /api/v1/employer/jobs/:slug/close`, plus **`GET /api/v1/employer/jobs`** — found
      missing while building the dashboard (7.3): there was no way to list an employer's own
      vacancies at all. New `Service.ListVacancies`/`JobRepository.ListMine`/`ListEmployerJobs`
      query (capped at 200, newest first, open and closed both — an MVP dashboard list, not
      the public catalogue); new requirement added to `employer-job-authoring`'s spec.
- [x] 6.4 Moderator endpoints: list pending claims, approve, reject. Plus an **admin**-only
      revoke endpoint (`POST /api/v1/employer/claims/:user_id/revoke`, built via
      `auth.RequireRole(queries, "admin")` locally rather than a new shared `mw.admin` field,
      since this is the only admin-gated route in the surface) — not in the original
      breakdown but required by `employer-account`'s spec ("An admin may revoke").
- [x] 6.5 Route registration (`RequireAuth` cookie-only via `mw.cookie`) and response shapes
      matching the `{"data": ...}`/`{"error": ...}` convention. `employer.ClaimMailer` (the
      mail port) exported, not package-private, so `Register`'s wiring can pass a properly
      nil-typed interface when SES is unconfigured — a nil concrete `*emailnotify.AuthMailer`
      through an unexported interface parameter would have smuggled in a non-nil interface
      wrapping a nil pointer, invisible to `Service.Claim`'s own guard (added as part of this
      task after the failure mode was reasoned through — see `employer.ErrMailUnavailable`).
      **Found and fixed one more real bug via the end-to-end integration test**:
      `user_email_codes.purpose`'s CHECK constraint (migration 0041) didn't allow the new
      `verify_work_email` purpose at all — every `Claim` call failed with a 500 until
      migration 0176 widened it, the same DROP/ADD pattern as the `closed_reason` migrations.
- [x] 6.6 **Found while designing the frontend, not in the original breakdown**: there was no
      way for a caller to read their OWN account before it activates — `GET /employer/company`
      was gated on `ActiveAccount`, so a pending/revoked claimant got a 403 with no way to
      even see their own status. Added `Service.MyAccount` (status regardless of
      pending/active/revoked, `ErrNotFound` only for no account at all) and re-pointed
      `GetCompany` to it; every WRITE stays `ActiveAccount`-gated exactly as before. New
      requirement added to `employer-account`'s spec.

## 7. Web (SvelteKit)

- [ ] 7.1 Claim flow pages: company name/search → work email → code confirmation → pending/
      active status
- [ ] 7.2 Company dashboard: curated profile edit form
- [ ] 7.3 Company dashboard: job list, create/edit/close forms
- [ ] 7.4 Moderator admin page addition for the pending-claims queue

## 8. Docs

- [x] 8.1 `internal/ingest/employer/AGENTS.md` (new substantial package), linked from the
      root `AGENTS.md` module table. Also updated `internal/ingest/AGENTS.md` (the block-level
      doc): added `employer` to the packages list and a 5th line to the "service-extraction
      seam" table (`employer -> identity/accounts`, the first `identity` import from `ingest`)
      — not in the original breakdown, but the same honesty-audit the existing 4-edge table
      already commits to.
- [x] 8.2 Updated `internal/identity/accounts/AGENTS.md` for the new generic `IssueCode`/
      `ConfirmCode` methods, the purpose constant, and the `user_email_codes.purpose` CHECK
      constraint gotcha (6.5's bug) — a future third-party purpose would hit the exact same
      500 without this note.

## 9. Verification

- [ ] 9.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean
- [ ] 9.2 `go vet -tags=integration ./...` clean; run the tagged integration suite for the
      touched packages (`internal/ingest/employer`, `internal/identity/accounts`,
      `internal/api/handler`, `cmd/import-yc`)
- [ ] 9.3 `pnpm check:sql` on the two new migrations
