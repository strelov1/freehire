## Why

A company on freehire is one person: `company_accounts` is keyed by user, a company slug can
be claimed once, and every vacancy and application is scoped to `created_by = actor`. The
moment a company hires through freehire with the applicant inbox
(`add-employer-applicant-pipeline`), the second recruiter or hiring manager has no way in —
they cannot claim the company (it is taken) and the claimant cannot share access short of
sharing a password. A hiring pipeline only one person can see is not one a company can use.

## What Changes

- **Invitations by email.** Anyone acting for an active company account invites a colleague
  at their work address; the colleague follows a single-use link (seven days) and joins
  under that address, whatever freehire account they are signed in with.
- **No roles.** Every member acts exactly as the claimant does: the applicant inbox and its
  pipeline, publishing, editing and closing vacancies, the company profile, the team. The
  claimant cannot be removed; anybody else can be, or can leave.
- **Ownership moves from the author to the team.** A vacancy and the applications to it
  belong to everyone acting for the company (`employer_team`), so a member sees and edits a
  colleague's vacancies; a removed member's vacancies are handed to the claimant.
- **One company per user** now holds across claimants and members.
- The profile page gains a Team section; an invitation lands on `/my/employer/join`.

- A member deleting their freehire account hands their vacancies to the claimant too (a
  database trigger, since account deletion lives below this block).

Not in this change: roles or permissions, transferring the account to another claimant
when the claimant deletes their freehire account (see design).

## Capabilities

### New Capabilities
- `employer-company-team`: a company account's members, how they join and leave, and what
  they may do.

## Impact

- Migration `0266`: `company_account_members`, `company_account_invitations`, function
  `employer_team`.
- `internal/ingest/employer` (`team.go`, team-scoped `BySlug` and URL guard),
  `internal/ingest/directapply` (own-company refusal), every employer-scoped query in
  `jobs_manual.sql` and `direct_applications.sql`.
- `internal/engage/emailnotify`: `TeamInviteMailer`.
- Six cookie-only routes under `/api/v1/employer/team`; `EmployerAccount.member`.
