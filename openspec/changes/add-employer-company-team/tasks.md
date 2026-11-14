## 1. Schema

- [x] 1.1 Migration 0266: `company_account_members`, `company_account_invitations`, `employer_team` (squawk clean)
- [x] 1.2 Employer-scoped queries in `jobs_manual.sql` and `direct_applications.sql` read `employer_team`
- [x] 1.3 Team queries (`company_team.sql`); claims refuse members; `make sqlc`

## 2. Domain

- [x] 2.1 `GetByUserID` reads members; `Account.Member`
- [x] 2.2 Team-scoped `BySlug`, `urlFree`, `Teammates`
- [x] 2.3 `team.go`: invite, withdraw, preview, accept, remove; unit tests
- [x] 2.4 directapply refuses applications from the vacancy's own company team
- [x] 2.5 Integration tests: membership, team vacancies, hand-over, one company per user, cap, resend, expiry, revoked account

## 3. Delivery and HTTP

- [x] 3.1 `TeamInviteMailer` with test and preview sample
- [x] 3.2 `typespec/employer.tsp` team routes, `EmployerAccount.member`; `make api-contract`; `gen:api-docs`
- [x] 3.3 Handlers and wiring; end-to-end handler test

## 4. Web

- [x] 4.1 API methods and types
- [x] 4.2 `EmployerTeam.svelte` on the profile page, with a spec
- [x] 4.3 `/my/employer/join`; the claim form points a colleague at the team
