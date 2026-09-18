## 1. Database schema

- [ ] 1.1 Migration: create `company_accounts` (`user_id` PK/FK → `users(id)` cascade,
      `company_slug` NOT NULL UNIQUE, `company_name` NOT NULL, `work_email` NOT NULL,
      `status` NOT NULL DEFAULT `'pending'` CHECK IN (`pending`,`active`,`revoked`),
      `verified_at` nullable, `created_at`/`updated_at`)
- [ ] 1.2 Migration: add `'employer_closed'` to `jobs_closed_reason_check`
      (`DROP CONSTRAINT IF EXISTS` → `ADD CONSTRAINT ... NOT VALID` → `VALIDATE CONSTRAINT`,
      matching migrations/0147, 0159, 0165)

## 2. `internal/identity/accounts`: generic code primitives

- [ ] 2.1 Add `PurposeVerifyWorkEmail` constant next to `PurposeVerifyEmail`/`PurposeResetPassword`
- [ ] 2.2 Add `Service.IssueCode(ctx, userID, purpose, email) error` and
      `Service.ConfirmCode(ctx, userID, purpose, code) error`, factored out of the existing
      private `issueCode`/`consumeCodeTx` (no side effect on confirm — caller decides what
      "verified" means)
- [ ] 2.3 Re-point `IssueVerificationCode`/`ConfirmVerification` to call the new generic
      methods (existing tests must stay green, unchanged behavior)
- [ ] 2.4 Add a `CodeMailer` method for the employer-claim verification email (distinct copy
      from the account-verification email) and implement it on the SES-backed mailer

## 3. `company_accounts` domain: claim and verification

- [ ] 3.1 sqlc queries: insert pending `company_accounts` row, get by `user_id`, get by
      `company_slug`, list `status='pending'`, activate, revoke, delete (reject) — run
      `make sqlc`
- [ ] 3.2 Small public-webmail-domain rejection list + a check function (gmail.com,
      outlook.com, yahoo.com, mail.ru, etc.)
- [ ] 3.3 Claim-start service: resolve slug via existing company search/`company_slug_aliases`
      or mint one via `normalize.CompanySlug`; insert pending row; refuse a user who already
      has an employer account; refuse a slug that already has one (conflict)
- [ ] 3.4 Claim-verify service: validate work email against the webmail blocklist, issue a
      code via `accounts.Service.IssueCode`
- [ ] 3.5 Claim-confirm service: confirm via `accounts.Service.ConfirmCode`; compare the
      email's domain to `companies.company_info->>'website'` when known; activate on match
      (seeding `company_info.website` if it was blank) or leave `pending` on mismatch/unknown
- [ ] 3.6 Moderator review service: list pending claims, approve (activate), reject (delete,
      freeing the slug)
- [ ] 3.7 Admin revoke action (sets `status='revoked'`, row and slug stay reserved)
- [ ] 3.8 Ownership guard helper: resolve the active `company_accounts` row for an actor,
      used by every employer-facing capability (profile edit, job create/update/close)

## 4. `internal/ingest/employer`: job authoring

- [ ] 4.1 Create: `Minter`-pattern wrapper around `moderation.Service.Create`
      (`source='employer'`, `Company` always the claimed `company_name`); pre-check via
      `GetJobBySourceExternalID(ctx, "employer", url)` refusing a different owner's URL (409)
- [ ] 4.2 Regression test: re-`Create` with the same URL by its own owner updates/reopens the
      existing vacancy rather than duplicating it
- [ ] 4.3 Regression test: a different employer cannot take over a vacancy via a colliding URL
- [ ] 4.4 New sqlc query: actor-scoped update (`WHERE public_slug = $slug AND created_by =
      $actorID AND source = 'employer'`) — run `make sqlc`
- [ ] 4.5 Update service: re-derive facets via `job.New(job.Draft{Input: jobderive.Input{...}})`
      directly (not via `moderation`'s private `derive()`); URL/company stay immutable
- [ ] 4.6 New sqlc query: actor-scoped close (`closed_at`, `closed_reason='employer_closed'`)
      — run `make sqlc`
- [ ] 4.7 Regression test: employer B cannot update or close employer A's vacancy (refused as
      not found)

## 5. `cmd/import-yc` guard

- [ ] 5.1 Add a guard so the upsert does not overwrite `year_founded`/`employee_count`/
      `hq_country`/`subindustry` for a company slug with an active `company_accounts` row
- [ ] 5.2 Regression test: an employer-asserted `year_founded` survives a YC-directory import
      run; a company with no employer account is unaffected (still overwritten as before)

## 6. API handlers

- [ ] 6.1 `POST /api/v1/employer/claim`, `POST /api/v1/employer/claim/verify`,
      `POST /api/v1/employer/claim/confirm`
- [ ] 6.2 `GET`/`PATCH /api/v1/employer/company` (curated profile fields only — never
      `company_types`/`company_sizes`, which stay job-derived)
- [ ] 6.3 `POST /api/v1/employer/jobs`, `PATCH /api/v1/employer/jobs/:slug`,
      `POST /api/v1/employer/jobs/:slug/close`
- [ ] 6.4 Moderator endpoints: list pending claims, approve, reject
- [ ] 6.5 Route registration (`RequireAuth` cookie-only) and response shapes matching the
      `{"data": ...}`/`{"error": ...}` convention

## 7. Web (SvelteKit)

- [ ] 7.1 Claim flow pages: company name/search → work email → code confirmation → pending/
      active status
- [ ] 7.2 Company dashboard: curated profile edit form
- [ ] 7.3 Company dashboard: job list, create/edit/close forms
- [ ] 7.4 Moderator admin page addition for the pending-claims queue

## 8. Docs

- [ ] 8.1 `internal/ingest/employer/AGENTS.md` (new substantial package), linked from the
      root `AGENTS.md` module table
- [ ] 8.2 Update `internal/identity/accounts/AGENTS.md` for the new generic `IssueCode`/
      `ConfirmCode` methods and purpose constant

## 9. Verification

- [ ] 9.1 `gofmt -l .`, `go vet ./...`, `go test ./...` clean
- [ ] 9.2 `go vet -tags=integration ./...` clean; run the tagged integration suite for the
      touched packages (`internal/ingest/employer`, `internal/identity/accounts`,
      `internal/api/handler`, `cmd/import-yc`)
- [ ] 9.3 `pnpm check:sql` on the two new migrations
