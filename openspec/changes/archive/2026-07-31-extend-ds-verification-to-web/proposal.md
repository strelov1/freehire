## Why

Every mechanical guarantee the design system owns stops at the package boundary.
`check:tokens` reads 15 files under `design-system/src/`; `validate:docs` reads the DSDS
entity JSON; the Storybook assertion reads `storybook-static`. None of them looks at
`web/`, which is where 216 Svelte files and every user-visible pixel live.

That asymmetry already produced the defect phase 7 was placed to catch: eleven primitives
were built, tested, storybooked and documented while unreachable from app code, with every
CI job green, because nothing measured the connection between the two halves. `export *`
fixed reachability. It did not make **use** measurable, and the same blind spot now hides
three more things:

- **Ten of fifteen primitives have zero call sites.** Alert, Avatar, Card, Chip, Dialog,
  EmptyState, FormField, Pagination, Table, Tabs, Tooltip are unused, while `web/` hand-rolls
  nine `role="dialog"` modals, six raw `<table>`s and five `role="tablist"`s.
- **186 raw-palette Tailwind utilities and 64 colour literals sit in `web/src`** — exactly
  the values `check:tokens` exists to reject, on the side of the boundary where the files are.
- **The three primitives the app actually depends on have no tests.** Button (47 files),
  Input (12), Badge (11) are the untested ones; the tested six are largely the unused ones.

## What Changes

- A **primitive adoption census**: a script that counts, per primitive, how many `web/src`
  files import it from `$lib/ui`, printed as a table and enforced against a committed
  baseline. The baseline is a ratchet — a count may rise, and may not fall.
- **`check:tokens` gains a second radius.** The existing package rule (no colour literal, no
  Tailwind arbitrary value) stays a hard failure for `design-system/src`. For `web/src` the
  same detectors run against a committed baseline count that may only decrease, because 250
  existing occurrences cannot be fixed in this change and a rule nobody can satisfy gets
  disabled rather than obeyed.
- **Tests for `button`, `input`, `badge`** — variant and size classes, the `disabled`/`type`
  contracts, and `cn` override precedence, matching the depth of the existing six.
- **CI asserts `design-system/dist` is in sync with `tokens/`.** The compiled
  `tokens-light.css` / `tokens-dark.css` are committed, `pnpm build` regenerates them, and
  nothing diffs the two — a token edit without a rebuild ships stale CSS silently.

Not in scope, and deliberately deferred to the next change: migrating the nine hand-rolled
modals onto `Dialog`, and introducing the semantic token the 84 amber utilities are standing
in for. This change makes those measurable; it does not perform them.

## Capabilities

### New Capabilities
- `design-system-verification`: the mechanical guarantees that keep the design system
  honest — what each one asserts, which files it reads, and how a guarantee that cannot yet
  be satisfied everywhere is expressed as a ratchet rather than a hard rule.

### Modified Capabilities
<!-- None. No existing spec describes the design system's verification; this is its first. -->

## Impact

- `design-system/scripts/` — `check-token-coverage.mjs` gains the web radius; a new
  `check-adoption.mjs`; two committed baseline files.
- `design-system/package.json` — one new script, one renamed intent.
- `design-system/src/*.test.ts` — three new test files.
- `.github/workflows/ci.yml` — the `design-system` job gains the adoption check and the
  `dist` diff. Both run in the existing job; no new job, no new install.
- `design-system/AGENTS.md` and `docs/verification.md` — each new guarantee named alongside
  the command that enforces it.
- No runtime code changes. No change to what the app ships.
