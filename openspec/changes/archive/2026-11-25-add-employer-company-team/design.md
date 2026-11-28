## Context

`company_accounts` (0174) holds one row per claimant with `UNIQUE (company_slug)` as the
claim race guard, and moderation and revocation address it by `user_id`. Vacancies are
`jobs` rows with `source = 'employer'` and `created_by` the author; every employer query
scoped them with `created_by = actor`, and the applicant inbox copied that predicate.

## Decisions

### 1. Members beside the account, not more accounts
`company_account_members (user_id PK, company_slug → company_accounts, work_email,
invited_by, joined_at)`. Making `company_accounts` many-rows-per-company would undo its claim
race guard and the address moderation uses. The claimant's row stays the account; a member
row says "acts for it". `user_id` as primary key keeps one company per member; the claim and
the acceptance each refuse the other table's users in their own statement.

### 2. One ownership rule, as a SQL function
`employer_team(actor) RETURNS SETOF bigint` — the claimant and members of the actor's
company. Every actor-scoped query reads `created_by IN (SELECT employer_team(actor))`, and the
Go-side checks ask `EmployerTeamIncludes`. One definition instead of a join repeated in nine
statements, and a later change to who belongs (roles, a transfer) edits one place. Status is
not in it: every capability checks `ActiveAccount` first, so a revoked account's members are
refused there.

The alternative — keeping `created_by = actor` and making a member act as the claimant's id —
was rejected: it would erase who published what and break `updated_by`.

### 3. An Account for a member is the company's, under the member's address
`GetByUserID` returns the account for claimants and members alike, with `Member` set and
`WorkEmail` the member's own: it is where applications to the member's vacancies are mailed
(`directapply` sends to the author's account) and the Reply-To of the candidate letters they
trigger. `UserID` stays the claimant's, the account's address in moderator and admin routes.

### 4. Joining by a mailed link
An owner or manager enters a colleague's work address. The recipient opens the mailed
256-bit link and accepts it from a verified matching Freehire account. Previously
issued invitations use link possession as proof of the address for their remaining
lifetime. Access is granted to an address the employer chose individually.
The token is stored only as SHA-256, consumed by the acceptance statement, valid seven days;
one live invitation per (company, address) — a resend replaces the token. Open invitations
are capped at 20 per company (each is a letter to an address the account typed). A failed
letter withdraws its invitation. The token travels to the API in a POST body, never a query
string, so it stays out of access logs.

### 5. Leaving hands vacancies to the claimant
`RemoveCompanyMember` deletes the member row and sets `created_by` of their employer
vacancies to the claimant in one statement. Left as is, a vacancy whose author is outside
every team stops taking applications (`directapply` resolves the receiving account from the
author) and disappears from the team's dashboard.

## Risks / Trade-offs

- **Account deletion by a member** would set `created_by` NULL through the existing foreign
  key, so a BEFORE DELETE trigger on `users` (`employer_hand_over_member_vacancies`) hands
  their vacancies to the claimant first. The account-deletion path lives in `identity`, which
  cannot import `ingest`, so the rule lives in the database.
- **Account deletion by the claimant** deletes the company account; its team and
  invitations cascade. The members' vacancies are detached first (`created_by` NULL, a
  BEFORE DELETE trigger on `company_accounts`), or they would follow a former member into
  the next company they act for. Handing the account to a member is a product decision left
  open.
- **A vacancy write racing a removal or a deletion** takes the author's users row, the
  company account and the membership `FOR KEY SHARE`, in the order a deletion takes them, so
  the removal waits and then hands over or detaches the new vacancy too, and the two never
  deadlock. An author who no longer acts for the company is refused.
- **Requests a removed member queued** (invite, withdraw, remove) recheck, under the
  company's lock, that their actor still acts for the company, and change nothing.
- **A claim and an acceptance racing for one user** each check a table the other writes, so
  both run under a per-user advisory lock (`LockEmployerActor`) in their own transaction.
- **A removed member's invitations** are withdrawn with them, so they cannot re-invite
  themselves before going.
## Recruiter access and activity

The account holder retains ownership. Migration 0298 gives existing members the
manager role with their existing company, vacancy, team and inbox permissions.
Previously issued invitations retain their original behavior until used, replaced
or expired. An invitation created or renewed after the upgrade grants recruiter
access and can be accepted only by an account with the matching verified work email.

Recruiters can list company vacancies and handle delivered applications, including
those on closed vacancies. They can download the submitted CV, record a review,
move the hiring stage and edit private team notes. They can see their own
membership and leave. The account holder and managers can manage the company
profile, vacancies, membership and invitations. All access requires an active
company account.

Migration 0297 adds the company activity log. Stage and note edits are recorded
in the same database statement as the change. Team changes record invitations,
withdrawals, joins and removals. Recruiters can read application events, and
managers can also read team events. The log omits applicant email addresses,
note text, invitation tokens, candidate messages and CV contents. Removing a
candidate deletes their application, its archived CV, notes and activity rows.
The actor is linked to their user ID and displayed by their current work
address while they remain in the company. Deleting a company account removes
its activity. Earlier actions have no activity rows.

Apply migrations 0297 and 0298 before starting the new server. The earlier
server treats every team member as a manager. Once recruiter access is in use,
a rollback must keep the role-aware authorization in place.
