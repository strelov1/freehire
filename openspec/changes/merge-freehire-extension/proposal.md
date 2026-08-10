## Why

`../freehire-extension` is a separate git repo (WXT + Svelte browser extension,
sidepanel job-application agent) whose only styling is hand-rolled CSS with
hardcoded hex colors — no design tokens, no shared components. It already talks
to hire's own backend (`internal/assistant`, `internal/browsertools`), and two
openspec specs (`extension-auth`, `extension-autofill`) already document its
backend-side contract from inside this repo. Moving the extension in lets it
consume `freehire-design-system` as a local package dependency, the way `web/`
already does, instead of drifting further from the product's visual language,
and keeps the client and its backend contract in one place.

## What Changes

- Import `../freehire-extension` into this repo as a new top-level package,
  `extension/`, as a fresh single commit (no `git subtree` — the source repo's
  51-commit history stays in the old repo, which gets archived on GitHub
  read-only, not deleted).
- Flatten the source repo's double nesting (`freehire-extension/extension/` →
  `extension/`) so `extension/package.json` sits at the package root, matching
  the `web/`, `design-system/` sibling layout.
- Add `freehire-design-system` as a `file:../design-system` dependency of
  `extension/` (no pnpm workspace migration needed — npm's `file:` protocol
  symlinks a local directory the same way pnpm's `link:` does; npm has no
  `link:` protocol of its own).
- Restyle the sidepanel — `App.svelte`, `MatchCard.svelte`, `JobDeck.svelte`,
  `JobDeckCard.svelte`, `ToolGroupList.svelte` — onto `freehire-design-system`
  components and tokens, replacing the hand-rolled CSS and hardcoded hex colors.
  **BREAKING** (extension-internal only): existing component class names/markup
  in the sidepanel change; nothing outside `extension/` depends on them.
- Carry over the extension's own `openspec/changes/*` (4 open changes) and
  `openspec/specs/*` (3 specs) into this repo's `openspec/` — no name
  collisions with existing entries. Drop the extension's `openspec/config.yaml`
  (identical to this repo's) and its local `.claude/skills/`,
  `.claude/commands/opsx/` (already available here via the superpowers/opsx
  plugins).
- Add `.github/workflows/extension.yml` (path-filtered to `extension/**`),
  adapted from the source repo's `build.yml` (`npm ci` / `npm test` / `npm run
  check` / `npm run zip`), as its own workflow file rather than folding into
  `ci.yml` — matching this repo's existing one-workflow-per-concern layout.
- Add `extension/AGENTS.md` as a domain doc (carried over from the source
  repo's `AGENTS.md`, trimmed of anything now redundant with this repo's root
  `AGENTS.md`) and a row for it in the root `AGENTS.md` module table.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
(none)

This is a repo-structure and implementation change — no product-facing
requirement changes. The sidepanel's behavior (what the agent does, what the
autofill/match-card features cover) is unchanged; only where the code lives and
how it's styled changes. `extension-auth` and `extension-autofill` already
describe the backend contract and are unaffected.

## Impact

- **New top-level package**: `extension/` (WXT + Svelte, npm-managed, builds
  independently of the Go/`web/` toolchains).
- **New dependency edge**: `extension/` → `design-system/` via `file:` (npm's
  local-symlink protocol — the same relationship `web/` has via pnpm's `link:`,
  different protocol name for the different package manager).
- **CI**: one new workflow file, no changes to existing ones.
- **Docs**: root `AGENTS.md` module table gains one row; `openspec/` gains the
  4 carried-over changes and 3 carried-over specs (folder-copied, unmodified).
- **External**: `freehire-extension` GitHub repo is archived (read-only) once
  the copy lands and CI on the new location is green — not deleted, not left
  actively pushed-to.
- **No backend changes.** `internal/assistant`, `internal/browsertools`, and
  the `extension-auth`/`extension-autofill` specs are untouched.
