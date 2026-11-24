# Auto-merge policy

The SuperPlane canvas "PR auto-merge policy" reads this file from `main` after the ready check
labels a pull request `ready-to-merge`. The principle: a pull request that passed the ready check
and every required check merges on its own, unless it touches a zone listed under "Always a
person". A pull request from a fork never auto-merges.

## Always a person

These win over everything below.

- This file, `.github/`, `AGENTS.md` and any `.agents/` file: they steer the agents and CI.
- `migrations/` and `deploy/`: production schema and rollback scripts; rollback does not undo a
  migration.
- Build and deploy configuration: `Dockerfile` files, `docker-compose.yml`, `Makefile`,
  `lefthook.yml`, `sqlc.yaml`, `ogen.yml`.
- `internal/identity/` (auth, accounts, billing, promo, account deletion), `internal/ai/llmkey/`,
  `internal/platform/aigateway/`: sessions, money, the paid-AI gate, API keys.
- CV and personal data handling (`internal/candidate/`, anything that reads CV text).
- A new dependency in `go.mod` or `package.json` (Dependabot bumps have their own workflow).
- A change that rewrites many existing production rows when it ships: a backfill, a recount or
  a data fix whose new logic changes stored values (for example a recount that moves thousands of
  companies to a different bucket). The ready check's comment usually names the effect.
- A pull request the ready check marked with an open question for the owner.

## Auto-merge

Every file not matched by "Always a person" is covered: Go and web code, Svelte components, copy
and i18n strings, SQL queries outside `migrations/`, tests, fixtures and documentation. A pull
request auto-merges when all of these hold:

- The latest `## Ready check` comment's verdict is **Ready** for the head commit being judged.
- Every required check is green on that commit.
- CodeRabbit has no open finding the ready check left unanswered.
- Size cap: at most 600 changed lines (additions plus deletions), not counting tests
  (`*_test.go`, `*.test.ts`, `*.spec.ts`, `web/e2e/`), `testdata/` fixtures and generated files.
- Generated files (sqlc output `*.sql.go`, `pnpm-lock.yaml`, `go.sum`) are covered only together
  with their source in the same pull request: the query file for sqlc output, a `package.json` or
  `go.mod` change for a lockfile. A generated file changed alone waits for the owner.
