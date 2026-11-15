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
- **A claim is only reviewable once its work email is confirmed.** The row is inserted at
  `Claim`, before the code is consumed, so `company_accounts.email_confirmed_at` (migration
  0181, written by `ConfirmClaim`) is what tells a claimant who proved the address from one
  who typed someone else's. The queue and `ActivateCompanyAccount` both require it — before
  it, 3 of the 4 pending claims on prod had never entered a code and Approve would have
  activated them. `ResendClaimCode` exists for the same reason: without it an expired code
  stranded the claim forever, since a user may hold only one account.
- **An academic domain never auto-activates** (`isAcademicDomain`): every student holds an
  address on the university's domain, so a match there proves enrolment, not the authority
  to hire. Subdomains are deliberately NOT treated as a match for the same reason —
  `pennmedicine.upenn.edu` is exactly where a student's address lives.
- **A vacancy's apply mode is its external id, and it never changes.** An `external`
  vacancy is keyed by its URL, as below. A `freehire` one (it takes applications on
  freehire, see [openspec/changes/add-in-app-apply](../../../openspec/changes/add-in-app-apply))
  has no URL: `createOnSite` mints `freehire:<uuid>` through `moderation.CreateInput.ExternalID`
  and `job.IsDirectApply` recognises it everywhere. Its questions are its `apply_forms` row
  (provider `freehire`, built by `applyform.FreehireForm`), replaced through
  `VacancyPatch.Questions`. The upsert's owner guard below never fires in that mode — it
  protects a URL two employers could both name, and a UUID is one nobody else can.
- **`CreateVacancy` guards a real gap in `UpsertManualJob`, inside the upsert itself.** Its
  `ON CONFLICT` would otherwise let employer B take over employer A's vacancy by resubmitting
  the same URL under `source='employer'`. The mint sets `moderation.CreateInput.Employer`
  (the caller and their company), and the conflict then updates only a row created by the caller's company
  team (`owner_team_of`, employer_team); for another company's row nothing is written and
  `moderation.ErrOwnedElsewhere` comes back, mapped to `ErrURLTaken`. A pre-check could not do
  this: two employers racing, or a member changing team between check and write, slipped
  past the old one. Re-creating under the same URL by the same TEAM is the (deliberate,
  no-separate-endpoint) reopen path; moderators leave the guard unset and may edit any
  manual job.
- **`UpdateVacancy`/`CloseVacancy` cannot reuse moderation's own `Update`/`BySlug`.** Those
  are scoped to "any manually-authored job" (`created_by IS NOT NULL`), correct for trusted
  staff and wrong for a self-service actor. `UpdateEmployerJob`/`CloseEmployerJob` add
  `created_by = ANY(ARRAY(SELECT employer_team(actor_id))) AND source = 'employer'` to the same shape.
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

## The company team (`team.go`, migration 0271)
- **A vacancy belongs to the company team, not to its author.** `employer_team(actor)` (a SQL
  function in 0271) is everyone who acts for the actor's company — the claimant and the
  members — and it is the ONE ownership rule: every actor-scoped vacancy query
  (`jobs_manual.sql`) and every inbox query (`direct_applications.sql`) reads
  `created_by = ANY(ARRAY(SELECT employer_team(...)))`, and the Go-side checks (`BySlug`,
  directapply's own-company refusal) ask `EmployerTeamIncludes`; a re-created vacancy is
  guarded inside the upsert (see `CreateVacancy` above). A new employer capability
  scoped to "the caller's vacancies" uses it too, never `created_by = actor`.
- **The claimant's row is still the account.** Its status, moderation, revocation and slug
  reservation are unchanged, and a revoked account refuses its members with it (they all go
  through `ActiveAccount`). `GetByUserID` returns the company's `Account` for a member too,
  with `Member` set and `WorkEmail` the member's OWN address — where their vacancies'
  applications go and what candidates reply to. `UserID` stays the claimant's: it is the
  account's address in the moderator/admin routes.
- **No roles.** Every member does everything the claimant does, including inviting and
  removing; the one exception is structural — the claimant is not a member row, so cannot be
  removed. Removing a member (or leaving) hands their vacancies to the claimant in the same
  statement (`RemoveCompanyMember`), because a vacancy created by somebody outside the team
  would stop taking applications and vanish from every dashboard.
- **One company per user, across both tables.** `company_account_members.user_id` is the
  primary key; a claim refuses a member (`InsertPendingCompanyAccount` writes no row) and an
  acceptance refuses a claimant (`AcceptCompanyInvitation`'s guard). The two checks read
  different tables, so each write runs in a transaction that first takes the user's
  `LockEmployerActor` advisory lock (`asActor`) — without it a concurrent claim and
  acceptance would each pass on a snapshot missing the other's row.
- **Joining is by link, not by domain.** The invitation is mailed to the address the team
  typed; following it is the proof the joiner reads that address, the role the claim's code
  plays. The token is 256 random bits, stored only as its SHA-256, single-use (the acceptance
  deletes the row it consumes) and expires in seven days. Re-inviting an address replaces its
  token, so a resend never leaves two working links; open invitations are capped at 20 per
  company because each is a letter freehire sends to an address the account typed. A letter
  that fails withdraws its invitation, so the list never shows somebody as invited who was
  never told. The joiner may accept under any freehire account — their sign-in address does
  not matter, the invited one is what they act under.
- **Every change to a team runs under the company's lock** (`LockEmployerCompany`, a
  transaction advisory lock): an invitation, a withdrawal and a removal (each rechecking,
  under the lock, that its actor still acts for the company — `asCompanyActor` — so a request
  a removed member queued changes nothing), and an acceptance (after the joiner's actor
  lock — always that order). So the 20-invitation cap cannot be passed by two invitations at
  once, and a member being removed cannot slip an invitation in after the removal withdrew
  theirs. Renewing an invitation bypasses the cap only while it is still live; reviving an
  expired one needs a free slot. A failed letter withdraws its invitation by token
  (`WithdrawToken`), so a resend that replaced the token meanwhile keeps its working link.
- **Removal also withdraws the member's invitations** — otherwise someone about to be
  removed could invite an address of their own and walk back in.
- **Removal locks the member row in its own statement first**
  (`LockCompanyMemberForRemoval`, FOR UPDATE): it waits for a vacancy write holding the row,
  and the hand-over statement after it then sees the new vacancy. Inside one statement the
  DELETE would wait but the hand-over would keep the earlier snapshot and miss it.
- **A member deleting their freehire account hands their vacancies over too**, by the
  `users_employer_hand_over` BEFORE DELETE trigger (0271). It must run before the row goes:
  `jobs.created_by` is ON DELETE SET NULL and that action fires before the member row's own
  cascade. Account deletion lives in `identity`, which cannot import this block, so the rule
  lives in the database.
- **An employer vacancy write holds its author's team rows.** With `CreateInput.Employer`
  set, moderation's `Create` takes the author's users row, the company-account row and the
  author's membership row, `FOR KEY SHARE` in its transaction (`LockAuthor`,
  `LockCompanyAccountBySlug`, `LockActingMembership`) — the order a user deletion takes them
  (the user, then the account and memberships it cascades to), so the two never deadlock. A removal or account
  deletion running meanwhile waits for the write to commit, then hands over or detaches the
  new vacancy too. An author who no longer acts for the company the write was prepared for
  (removed, the account gone, or moved to another company) is refused
  (`moderation.ErrNoCompany` → `ErrNotActive`).
- **Known limit:** a CLAIMANT deleting their freehire account deletes the company account,
  and its team and invitations cascade with it; its vacancies stop taking applications, as
  before teams existed. The members' vacancies are detached first (`created_by` NULL, the
  `company_accounts_detach_team` trigger, 0271), as the claimant's own are by the foreign key —
  left pointing at a member, they would follow them into the next company they act for, along
  with their applications and CVs. Handing the account to a member is a product decision not
  yet made.

