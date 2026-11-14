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
The team types a work address; a link carrying a 256-bit token goes there; whoever follows
it signed in may join, and acts under the invited address. Possession of the link is the
proof of the address, exactly as the claim's code is. Not by domain: a verified domain does
not tell freehire who on it may hire, and public and academic domains would need exceptions.
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
- **No roles** means any member can remove any other. Chosen deliberately for now; roles
  would add a column to `company_account_members` and a check in `RemoveMember` and the
  vacancy writes.
