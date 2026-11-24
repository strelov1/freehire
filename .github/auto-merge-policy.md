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

<!-- TODO(owner): which changes may ship without you? One rule per line, e.g.
     "- Only `docs/`, `*.md` outside `.github/` and `.agents/`."
     Decide on: docs, tests-only changes, web copy and styles, a size cap
     (pr-size already blocks the huge ones), and which packages you trust the agents with. -->
