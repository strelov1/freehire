## Context

`design-system/scripts/` holds three verification scripts, and all three read only files
inside the package: `build-tokens.mjs` (collision assertion), `validate-docs.mjs` (DSDS entity
paths and token lists), `check-token-coverage.mjs` (no colour literal, no Tailwind arbitrary
value in `src/*.svelte`). CI's `design-system` job runs them with `working-directory:
design-system` against a full repo checkout, so `../web` is on disk and simply never looked at.

The package side is close to airtight — 15 components, every guarantee a command. The
consumer side has 216 Svelte files and no guarantee at all. Phase 7 showed what that costs:
the two halves were disconnected for three phases with every job green, because each job
proved its own half built and nothing proved they met.

Two facts constrain the design:

- **`web/src/app.css` imports `freehire-design-system/theme.css` directly, and must.** That is
  the documented CSS contract. A "the app never names the package" rule has to be about
  component imports, not `@import`.
- **The existing detectors do not cover web's actual problem.** `text-amber-600` is neither a
  literal nor an arbitrary value — it is a well-formed Tailwind utility off the built-in
  palette, invisible to `COLOUR` and `ARBITRARY` alike. In the package it never arose. In
  `web/` it is the majority of the violations.

## Goals / Non-Goals

**Goals:**

- Make primitive adoption a number CI reads, so an unused primitive is visible and a
  regression in use is red.
- Extend the token rule to `web/src` without pretending 250 existing occurrences are a bug
  to fix today.
- Test the three primitives the app actually depends on.
- Make a stale `dist/` impossible to commit.

**Non-Goals:**

- Fixing any of the counted violations. Every baseline lands at whatever today's number is.
- Migrating the nine hand-rolled modals, or adding the semantic token the amber utilities
  stand in for. Both are the next change; this one gives them a scoreboard.
- Any runtime behaviour change. Nothing here alters a byte the app ships.
- Counting *how often* a primitive is used. Breadth (files that import it) is the metric;
  volume is not.

## Decisions

### One set of detectors, two radii — not two scripts

`check-token-coverage.mjs` gains a `web/src` pass rather than a sibling script gaining a
second copy of `COLOUR` and `ARBITRARY`.

Two copies of a regex is precisely the failure mode this change exists to close: the DSDS
`props` and the Storybook `argTypes` are each a hand-kept second copy of a primitive's
variants, and both have already drifted. A forked detector would drift the same way, and the
drift would be silent — one radius quietly stops catching what the other still does.

*Alternative considered:* a script under `web/` run by web's CI job. Rejected for the fork.
*Alternative considered:* export the detectors from the package and import them in web's
script. Rejected — `scripts/` is not in `exports`, and adding it there makes internal
tooling part of the package's public surface to buy nothing.

The two radii are not the same rule, and the script says so:

| | `design-system/src` | `web/src` |
|---|---|---|
| colour literal | hard fail | ratcheted |
| Tailwind arbitrary value | hard fail | ratcheted |
| raw palette utility | — | ratcheted |

The package radius keeps zero tolerance: 15 files, currently clean, and a new violation there
is always a mistake. The web radius cannot be zero today, so it is a number.

### A third detector, for the web radius only

`PALETTE` matches `(bg|text|border|ring|fill|stroke|from|via|to|decoration|outline|shadow|
accent|caret|divide|placeholder)-<tailwind-hue>-<shade>`. It runs only over `web/src`.

It is not added to the package radius because it would be dead weight there — the primitives
have no palette utilities and `check:tokens` already fails them on anything unowned. It stays
a web concern until a primitive ever needs it, at which point the radius table is where it
gets turned on.

### The ratchet: an exact number, and a `--update` flag

A baseline is the exact count, and **any** difference fails — a rise lists the offending
lines, a fall says the baseline is stale and to run `--update`.

*Alternative considered:* fail only on a rise, pass silently on a fall. Rejected: the baseline
then sits at 250 while reality is 40, and a regression from 40 back to 250 is green. A ratchet
that only ever loosens is a ratchet that eventually asserts nothing — the same shape as a
`verification.md` written as prose, which was wrong in eight days.

*Alternative considered:* CI rewrites the baseline and pushes. Rejected: it buys one saved
command in exchange for giving CI write access to a repo whose main branch is protected for
admins too.

The cost is one extra command on the PR that improves things. The return is that every
improvement shows up as a line in the diff — `"dialog": 0 → 3` is the change's own evidence.

The comparison is `===` in both directions, so the two ratchets differ only in the message
they print. That makes the shared piece small and worth sharing: `scripts/ratchet.mjs` owns
read-compare-update-report. It is extracted not to avoid typing thirty lines twice, but
because the *semantics* — exact, both directions, exit non-zero — is a contract, and a second
copy is free to weaken to `>=` without anyone noticing.

### Adoption is counted by import, and the `$lib/ui` door is enforced

`check-adoption.mjs` walks `web/src`, and for each `.svelte`/`.ts` file collects the names
imported from `$lib/ui`, handling multi-line specifier lists and `X as Y` aliases. The count
per primitive is **files that import it** — one file using `Button` twenty times counts once.

Breadth, not volume, because the question the metric answers is "does the app reach for this
primitive when it needs one", and a file that reaches for it once has already answered yes.
Volume would also make the number swing on ordinary refactors that split or merge a component.

The same walk enforces the door: a `.svelte` or `.ts` file under `web/src` that imports
`freehire-design-system` by name is a hard failure, not a ratchet, because there are zero
today and `$lib/ui` exists precisely so there stay zero. `app.css`'s `@import` is untouched —
the walk only reads JS and Svelte import statements.

The census also names the primitives at zero, out loud, every run. That is the finding this
change was written for and it should not need a script rerun to see.

### `dist` is checked by rebuilding and diffing

`"check:dist": "pnpm build && git diff --exit-code -- dist"`.

Style Dictionary emits no timestamp or generator header — verified against the committed
`tokens-light.css` — so a rebuild of unchanged sources is byte-identical and the diff is
deterministic. The script rebuilds rather than assuming a prior build, so it is correct run
alone; the duplicate build costs about a second.

*Alternative considered:* stop committing `dist/` and build it on demand. Rejected as out of
scope and larger than it looks: `web` resolves the package through `file:../design-system`,
and the release path has already been broken twice by that link's build state. Changing when
`dist` exists is a deploy-shaped decision, not a verification one.

### The three tests

`button`, `input`, `badge` — the primitives with 47, 12 and 11 consuming files and no test
between them. Scope matches the existing six: the contract a consumer depends on, and the
regression that would otherwise be silent. Concretely, each variant and size resolving to
distinct classes (Button's `destructive` is the one that has already gone missing from a
hand-kept list), the native attribute passthrough, and `class` winning over the base classes
through `cn` — the tailwind-merge precedence every call site assumes and nothing asserts.

## Risks / Trade-offs

- **A verification script in `design-system/scripts/` reads `../web/src`.** The package is
  `private`, unpublished, and linked by path, so there is no packaging consequence today. But
  it is a real seam: if the package is ever extracted, `check-adoption.mjs` and the web radius
  stay with the repo, not with the package. → Both are named as repo-boundary checks in their
  own headers, not as package checks.
- **A ratchet is a rule nobody has to satisfy.** 250 grandfathered occurrences can sit there
  forever and the build stays green. → It is still strictly better than the status quo of no
  number at all, and the next change spends the budget it makes visible. The exact-match
  design at least guarantees the number is true.
- **`PALETTE` will have false positives.** Not every `text-amber-600` in `web/` is a design
  system failure. → The ratchet does not require adjudicating them now; it freezes the count.
  When the count is small enough to argue about, an `ALLOWED`-style exception list — which
  already exists in this script and already asserts its own entries still match — is where a
  legitimate one goes.
- **Baselines are whatever the script reports, not what this document says.** The figures in
  the proposal came from ad-hoc greps with narrower patterns than the real detectors. → First
  run writes them; the committed numbers are the script's, and the task list says so.
