# Auto-merge policy

The SuperPlane canvas "PR auto-merge policy" reads this file from `main` after the ready check
labels a pull request `ready-to-merge`. It turns on auto-merge only when a rule under
"Auto-merge" covers every changed file and no rule under "Always a person" matches. Anything
else, or any doubt, waits for the owner. A pull request from a fork never auto-merges.

## Always a person

These win over any rule below.

- This file, `.github/`, `AGENTS.md` and any `.agents/` file: they steer the agents and CI.
- `migrations/`: production schema; rollback does not undo a migration.
- `internal/identity/` (auth, accounts, billing, promo, account deletion), `internal/ai/llmkey/`,
  `internal/platform/aigateway/`: sessions, money, the paid-AI gate, API keys.
- CV and personal data handling (`internal/candidate/`, anything that reads CV text).
- A new dependency in `go.mod` or `package.json` (Dependabot bumps have their own workflow).
- A pull request the ready check marked with an open question for the owner.

## Auto-merge

- Documentation: `docs/` and `*.md` files anywhere, except `.github/`, `.agents/` and `AGENTS.md`
  (those stay under "Always a person").
- Tests only: Go `*_test.go` and `testdata/` fixtures; web `*.test.ts`, `*.spec.ts`, `*.test.mjs`
  and everything under `web/e2e/`. A test-only pull request touches no non-test file: test
  configs (`vitest.config.ts`, `playwright*.config.ts`, `vitest-stubs/`) are not covered.
- Web styles: stylesheet files only (`web/src/app.css`). Svelte components and copy or i18n
  strings are not covered yet, even when the change is only a class name or a string.
- Size cap on every rule: at most 300 changed lines (additions plus deletions) in total. A bigger
  pull request waits for the owner even when every file is covered.
- A pull request mixing covered and uncovered files waits for the owner. Generated files (sqlc
  output, lockfiles) are not covered by any rule.
