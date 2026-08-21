# Verification

What the design system guarantees, and the check that holds each guarantee up.

The phase 7 pass that opened this file recorded its results as prose — a table of
warning counts and a claim of zero hardcoded colours. Eight days later the counts
were wrong in both directions and the colour claim was false, with nothing red to
say so. A guarantee written down is a guarantee that rots, so each one below names
the command that enforces it. All of them run in CI on every PR.

| Guarantee | Enforced by | Reads |
|---|---|---|
| The tokens compile to a light and a dark stylesheet | `pnpm build` | `tokens/` |
| The committed `dist/` is the output of the committed sources | `pnpm check:dist` | `tokens/`, `dist/` |
| Every primitive type-checks | `pnpm check` | `src/` |
| The primitives behave (`src/*.test.ts`) | `pnpm test` | `src/` |
| The verification scripts behave (`scripts/*.test.mjs`) | `pnpm test` | `scripts/` |
| The DSDS entities still describe the package they document | `pnpm validate:docs` | `docs/dsds/`, `src/` |
| Nothing styles itself outside the token scale | `pnpm check:tokens` | `src/`, **`../web/src`** |
| The app reaches for the primitives, and through one door | `pnpm check:adoption` | `src/index.ts`, **`../web/src`** |
| Every story builds | `pnpm build-storybook` | `src/*.stories.ts` |
| The built Storybook actually carries the primitives' utilities | CI, `grep .animate-pulse storybook-static` | `storybook-static/` |

The two in bold cross the package boundary. They are **repo checks that live here**,
not package checks: they read a directory the package itself knows nothing about,
and if `design-system/` is ever extracted they stay with the repo.

## What an entity has to keep true

An entity in `docs/dsds/` is a hand-written copy of something in `src/`, so every
field of it is free to drift. `scripts/validate-docs.mjs` compares three of them
back against the package: the files an entity points at exist, its `tokens` list
matches the token file's keys, and its `props` list matches what the component
destructures out of `$props()`. Each comparison runs **both ways** — a copy that
omits half the original is as wrong as one that invents a field, and only the first
kind is the sort of drift a reader would never notice.

Props are named the way a **call site** writes them, not the way the component
destructures them: `class: className` is documented as `class`, and the rest spread
is documented as `...rest` so the passthrough has somewhere to be described. This is
the check with a reader in mind — an agent decides what it may pass from the `props`
list alone. `Button` grew `target` and `rel` in #1920 and the entity never learned
about them, so the fact that the component fills in `rel="noopener noreferrer"` for a
`_blank` target lived only in a source comment. Nothing was red for the week it took
this check to notice.

## Token coverage — two radii

`scripts/check-token-coverage.mjs` looks for three things, all the same defect: a
value the theme cannot move and the `.dark` selector cannot override.

| Detector | Example |
|---|---|
| colour literal | `#fff`, `rgb()`, `oklch()` |
| Tailwind arbitrary value | `p-[7px]`, `bg-[#fff]` |
| raw palette utility | `text-amber-600`, `bg-emerald-500` |

No compiler objects to any of them, so without this the build stays green and the
thing silently stops following the theme.

**`src/` — hard zero.** Fifteen files, currently clean, so a violation is always a
mistake. The palette detector does not run here: the primitives have none, and
`check` would fail them on anything unowned anyway.

**`../web/src` — held per file.** 216 files carrying 550 violations across 106 of
them. No single change removes that, and a rule nobody can satisfy is a rule that
gets switched off rather than obeyed — so each file is pinned at its current count
in `scripts/web-token-baseline.json`. Per file rather than per total, so a
regression names the file and prints its lines, and so the baseline doubles as the
ranked list of what is left to fix.

One definition per detector, shared by both radii. A second copy is how the DSDS
props and the Storybook `argTypes` drifted from the components they describe, and a
forked detector drifts silently — one radius quietly ceasing to catch what the
other still does.

Arbitrary *variants* pass — `[&_tr]:border-b` in `table.svelte` is a selector, and
no token could stand in for it. The `:` after the bracket is the whole
discriminator.

One deliberate exception, listed in the script and failing if it ever stops
applying: `avatar.svelte` derives a per-name hue as an inline `hsl()` pair. A token
per hue would be 360 tokens, and the two lightnesses are fixed so the pair carries
its own contrast in either theme without an override.

## Reachability, and use

Every check above establishes that the package is correct. None of them
establishes that the app can reach it, and that gap was real:
`web/src/lib/ui/index.ts` enumerated four primitives out of fifteen, and the other
eleven were built, tested, storybooked and documented while no app code could
import them — with all of the above green.

It is now `export * from 'freehire-design-system'`, so the seam cannot drift again.
That is a structural guarantee rather than a check, which is why it has no row in
the table.

Reachability is not use, though, and `pnpm check:adoption` is what measures the
difference. It counts the `web/src` files that import each primitive, holds every
count at `scripts/adoption-baseline.json`, and names the unused primitives on every
run whether it passes or fails. Today:

```
adoption: 4/15 primitives have a consumer in web/src
  unused: Alert, Avatar, Card, Chip, Dialog, EmptyState, FormField,
          Pagination, Table, Tabs, Tooltip
```

Files, not occurrences — the question is whether the app reaches for a primitive
when it needs one, and a file that reaches once has answered it. The primitive list
comes from `src/index.ts`, so a new primitive joins the census by itself.

The same walk enforces the door. `$lib/ui` exists so app code never names the
package, there are zero violations today, and zero is a number a wall can hold — so
a `.svelte` or `.ts` file under `web/src` importing `freehire-design-system` fails
outright, no baseline and no exception. `web/src/app.css`'s `@import` of the
package's `theme.css` is untouched: it is the CSS contract every consumer must
import, and the walk reads only `from`-clauses.

## The ratchet

Both baselines are held by `scripts/ratchet.mjs`, and it is **exact in both
directions**. A regression fails. An improvement fails too, and asks to be recorded:

```
✗ adoption: Dialog: 0 → 3 — improved; rerun with --update to record it
```

`--update` rewrites the baseline; commit the diff. That extra step is the price of
the number staying true. A ratchet that absorbs improvements silently sits at 550
while reality is 40, and the regression from 40 back to 550 passes — green, and
asserting nothing. Which is where this file came in.

A baseline entry nothing measures any more fails as well, the way the token
allowlist fails an exception that no longer applies: an allowance that outlives its
reason is an allowance covering the next violation.
