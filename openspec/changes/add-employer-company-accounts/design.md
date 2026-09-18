## Context

See `proposal.md` for motivation. The constraints that shape this design come from reading
the existing code, not from a green field:

- `companies` is derived from `jobs` (`SyncCompaniesFromJobs` writes only `slug`+`name`;
  `DeleteOrphanCompanies` deletes any non-`is_reference` row with no job). `company_slug_aliases`
  is the one existing company-adjacent table that deliberately sits outside that derivation
  (`docs/agents/company-identity.md`) — `company_accounts` follows the same shape.
- `companies.company_types`/`company_sizes` are job-derived (`RefreshCompanyFacets`, from
  `jobs.enrichment`), not curated. The genuinely curated fields are exactly the ones
  `cmd/import-yc`'s `UpsertYCCompany` owns: `tagline`, `company_info.description`,
  `company_info.website`, `industries` (via the existing `SetCompanyIndustries`),
  `year_founded`, `employee_count`, `hq_country`, `subindustry`.
- `internal/ingest/moderation` (moderator-authored jobs) and `internal/ingest/submission`
  (public submission queue, which mints through `moderation.Service.Create` via a `Minter`
  interface) are the two existing manual-intake write paths. `moderation.Service.Update`
  and its `BySlug` are scoped only to "any manually-authored job" (`created_by IS NOT NULL`),
  never to a specific actor — confirmed by reading `internal/ingest/moderation/repository.go`
  and the `UpdateManualJob` SQL — so they are not safe to reuse for a self-service actor.
- `UpsertManualJob`'s `ON CONFLICT (source, external_id) DO UPDATE` does not check
  `created_by` before overwriting a row's content (only `updated_by` is reassigned). Reusing
  `moderation.Service.Create` unguarded for employer-authored jobs would let one employer
  take over another's posting by resubmitting the same URL.
- `internal/identity/accounts` already owns a generic, purpose-keyed mailed-code mechanism
  (`codes.go`), but only exposes it through two account-specific wrappers
  (`IssueVerificationCode`/`ConfirmVerification`) that hardcode a side effect
  (`MarkEmailVerified`).
- Layering: `internal/ingest` is layer 7, `internal/identity/accounts` is layer 3; a block
  may import any block strictly below it, so `internal/ingest/employer` importing
  `internal/identity/accounts` is allowed.

## Goals / Non-Goals

**Goals:**
- A verified employer can manage their own company's curated profile and publish, edit, and
  close their own vacancies, without a moderator in the ordinary path.
- Reuse every existing write path, dictionary, and code-verification mechanism that already
  does the relevant job correctly; add new code only where an existing mechanism is
  genuinely the wrong shape (actor-scoped edit/close) or has a real gap (the URL-collision
  takeover, the `cmd/import-yc` overwrite).
- Leave the data model able to carry the later, explicitly out-of-scope work (exclusive
  postings, in-house applications) without a second migration to introduce the actors
  involved (`company_accounts`, `jobs.source = 'employer'`).

**Non-Goals:**
- Claiming or editing a company's existing crawler-ingested postings.
- Team/multi-seat access to one company account.
- Any billing/paywall gate.
- Exclusive postings or in-house application intake (future work; only the identity
  groundwork is laid here).
- Opening employer endpoints to API-key auth (cookie-only for this change; an easy later
  addition once the surface is proven).

## Decisions

**`company_accounts` is a new, non-derived table, not a `users.role` value.** `users.role`
is a closed, three-value CHECK-constrained enum (`user`/`moderator`/`admin`) describing
site-wide staff privilege, not "owns company X" — squeezing company ownership into it would
also make it exclusive with moderator/admin status, which is wrong. A separate table mirrors
the precedent `company_slug_aliases` already set for state that must outlive
`SyncCompaniesFromJobs`/`DeleteOrphanCompanies`.

**`UNIQUE(company_slug)` on `company_accounts`, reserved at first claim (status `pending`).**
This makes the double-claim race a plain unique-constraint conflict rather than a hand-built
lock, and means a claim that never completes verification still blocks a second claim on the
same company — accepted as the simplest correct behavior for this MVP; a stale pending claim
is visible to and resolvable by a moderator, not silently stuck.

**Create reuses `moderation.Service.Create` via the `Minter` interface `submission.Service`
already uses; Update/Close are new, actor-scoped code.** Create's shape (validate, derive,
upsert, enqueue enrichment + search) is identical for a moderator and an employer; the only
difference is authorization and the fixed company identity, both enforced before the
delegate call. Update/Close cannot be shared because the existing queries are scoped to "any
manually-authored job," and widening that scope would open the exact same gap for the
moderator path this change is fixing for the employer path. Alternative considered: give
`moderation.Service.Update` an optional actor-scope parameter. Rejected — moderator edits are
deliberately allowed to touch any manually-authored job (staff trust model), and threading an
optional scope through would make every existing call site carry a parameter that is always
`nil` today, for the benefit of a caller that does not yet exist in that package.

**The URL-collision guard is a pre-check using the existing `GetJobBySourceExternalID`
query, not a new constraint.** Before delegating to the `Minter`, look up
`(source='employer', external_id=url)`; refuse (409) if found with a different
`created_by`, otherwise proceed (which also covers the legitimate re-create/reopen case for
the same owner). A DB-level fix (e.g., a trigger) was considered and rejected as
disproportionate — the guard only needs to run on the one write path that newly introduces
multiple mutually-untrusted actors sharing a `source` value.

**Facet derivation for the new Update path calls `job.New(job.Draft{...})`/`jobderive.Input`
directly — the same exported primitives `moderation.go`'s private `derive()` wraps —
rather than exporting `derive()` itself.** This keeps `docs/agents/company-identity.md`'s
"every write path shares it" invariant intact (the shared thing is `jobderive`, not
`moderation`'s private convenience wrapper) without adding an export to a package whose
`derive()` is intentionally private to its own `CreateInput`/`UpdatePatch` shapes.

**`internal/identity/accounts` gains two generic methods (`IssueCode`/`ConfirmCode`) and a
purpose constant, factored out of the already purpose-parameterized private
`issueCode`/`consumeCodeTx`.** `IssueCode` takes delivery as an explicit
`send func(ctx, email, code) error` parameter rather than a `CodeMailer` method, so it needs
no change to that interface at all; `internal/ingest/employer` defines its own tiny mailer
port and passes its method straight through. `IssueVerificationCode` becomes a thin wrapper
over `IssueCode`. `ConfirmCode`, found during implementation to be a real exception, is
NOT what `ConfirmVerification` calls: `ConfirmVerification` bundles the code-consume and
`MarkEmailVerified` in one transaction (accounts' own "spend the code + write the value, or
nothing" invariant), and a `ConfirmCode` that commits on its own would split that atomicity
if `ConfirmVerification` tried to wrap it. `ConfirmCode` is a standalone sibling instead,
sharing only the private `consumeCodeTx` both call. Alternative considered: build a separate,
parallel code-issuing mechanism inside `internal/ingest/employer`. Rejected — it would
duplicate the rate-limiting/hashing/attempt-bounding transaction logic that already exists
and is already tested, for no benefit.

**A verified employer's profile writes are authoritative (overwrite), not fill-gap.** Every
existing company-info writer (`cmd/import-yc`, the Wikipedia backfill, ingest's
adapter-supplied description) treats itself as one of several possibly-wrong sources and
therefore never overwrites another's value. An employer editing their own company is the
subject speaking about itself — strictly more authoritative than any inferred or imported
source — so its write should win outright. This needs one small protective addition:
`cmd/import-yc` unconditionally overwrites `year_founded`/`employee_count`/`hq_country`/
`subindustry` on conflict today (its `tagline`/`company_info` are already fill-gap-protected,
and `industries` already unions), so it gains a guard against clobbering those four columns
for a company with an active employer account. The Wikipedia backfill needs no change — its
candidate query already excludes any company with a non-blank `tagline`.

**No separate `reopen` endpoint.** Re-`Create`-ing under the same URL, by the same owner,
already reopens a closed posting (`UpsertManualJob`'s conflict branch clears `closed_at`) —
this is the existing moderator behavior too, and giving the employer flow a second way to
express the same action would be two paths to keep in sync for no benefit.

## Risks / Trade-offs

- **[Risk]** Domain-match verification is exact-string, not fuzzy — a typosquatted
  lookalike domain will not auto-activate, but neither will a company's legitimately
  secondary domain (a second brand, a regional TLD). → **Mitigation**: falls to moderator
  review rather than being refused; not a dead end, just a one-step-slower path.
- **[Risk]** Single point of failure: one user per company account, no self-service
  handoff if that person leaves. → **Mitigation**: accepted scope limitation for this MVP
  (see proposal); an admin can revoke, and the real owner can then re-claim — a manual,
  moderator-mediated escape hatch, not an automated one.
- **[Risk]** A verified employer's authoritative overwrite could, in principle, let a bad
  actor who slipped through verification plant incorrect `year_founded`/`hq_country`/etc.
  → **Mitigation**: the same verification gate (domain match or moderator review) that
  gates vacancy publishing gates profile edits; there is no separate, weaker check for
  profile-only edits.
- **[Trade-off]** `internal/identity/accounts` gains new public surface
  (`IssueCode`/`ConfirmCode`) used by exactly one caller today. Accepted because the
  alternative (duplicating the transactional code logic) is strictly worse, and the new
  methods are general enough that a future purpose-keyed verification need (not yet
  identified) would reuse them too.

## Migration Plan

1. Migration: create `company_accounts` (see proposal/specs for the shape).
2. Migration: add `'employer_closed'` to `jobs_closed_reason_check`, following the existing
   `DROP CONSTRAINT IF EXISTS` → `ADD CONSTRAINT ... NOT VALID` → `VALIDATE CONSTRAINT`
   pattern (see migrations/0147, 0159, 0165).
3. `internal/identity/accounts`: add the purpose constant and the two generic methods (no
   `CodeMailer` change needed — see Decisions).
3a. **Found only once the end-to-end HTTP test ran against real Postgres**: migration to
    widen `user_email_codes.purpose`'s own CHECK constraint (added in 0041, before this
    package existed) to admit `'verify_work_email'` — every `Claim` call 500'd without it.
    Same DROP/ADD pattern as 2, sized for a small, short-lived table (no `NOT VALID` split
    needed). No fake-repository unit test could have caught this; it lives entirely in a
    constraint neither `internal/identity/accounts` nor `internal/ingest/employer` declares.
4. New `internal/ingest/employer` package (service + repository + sqlc queries for the
   actor-scoped update/close, the public-webmail-domain check, and the authoritative
   curated-profile write — `SetCompanyAccountProfile`, nil-means-unchanged via `sqlc.narg`).
5. `cmd/import-yc`: add the `company_accounts`-existence guard on the four columns.
6. `internal/api/handler`: new `/api/v1/employer/*` routes (claim/confirm, curated-profile
   GET/PATCH, job create/edit/close), the moderator queue (list/approve/reject), and an
   admin-only revoke route.
7. `web/`: claim flow pages, company dashboard (job list, profile edit form).

No backfill and no data migration of existing rows is needed — this change only adds new
tables/columns and new write paths; nothing about existing jobs or companies changes shape.
Rollback is an ordinary migration-down plus removing the new routes/package; no existing
write path is altered in a way that needs its own rollback story.

## Open Questions

- Whether `source = 'employer'` should be added to the frontend's generated `SOURCE_VALUES`
  (making it a filterable value on `/jobs`) is left for later — it needs no schema or backend
  change either way, and can be decided once there is enough employer-published volume for a
  dedicated filter to be useful.
- The exact moderator-review UI treatment (a new tab on the existing submissions/boards
  moderation surface vs. a dedicated page) is left to implementation; it does not affect the
  spec-level behavior in `employer-account`.
