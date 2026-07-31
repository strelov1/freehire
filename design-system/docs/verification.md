# Verification

What the package guarantees about itself, and the check that holds each guarantee up.

The phase 7 pass that opened this file recorded its results as prose — a table of
warning counts and a claim of zero hardcoded colours. Eight days later the counts
were wrong in both directions and the colour claim was false, with nothing red to
say so. A guarantee written down is a guarantee that rots, so each one below names
the command that enforces it. All of them run in CI on every PR.

| Guarantee | Enforced by |
|---|---|
| The tokens compile to a light and a dark stylesheet | `pnpm build` |
| Every primitive type-checks | `pnpm check` |
| The primitives behave (`src/*.test.ts`) | `pnpm test` |
| The DSDS entities still describe the package they document | `pnpm validate:docs` |
| No primitive styles itself outside the token scale | `pnpm check:tokens` |
| Every story builds | `pnpm build-storybook` |
| The built Storybook actually carries the primitives' utilities | CI, `grep .animate-pulse storybook-static` |

## Token coverage

`scripts/check-token-coverage.mjs` fails on a colour literal (`#fff`, `rgb()`,
`oklch()`) or a Tailwind arbitrary value (`p-[7px]`, `bg-[#fff]`) in
`src/*.svelte`. Both are the same defect: a value the theme cannot move and the
`.dark` selector cannot override. Neither compiler objects to one, so without this
the build stays green and the primitive silently stops following the theme.

Arbitrary *variants* pass — `[&_tr]:border-b` in `table.svelte` is a selector, and
no token could stand in for it.

One deliberate exception, listed in the script and failing if it ever stops
applying: `avatar.svelte` derives a per-name hue as an inline `hsl()` pair. A token
per hue would be 360 tokens, and the two lightnesses are fixed so the pair carries
its own contrast in either theme without an override.

## Reachability

Every check above establishes that the package is correct, not that the app can
reach it. That gap was real: `web/src/lib/ui/index.ts` enumerated four primitives
out of fifteen, and the other eleven were built, tested, storybooked and documented
while no app code could import them — with all of the above green.

It is now `export * from 'freehire-design-system'`, so the seam cannot drift again.
That is a structural guarantee rather than a check, which is why it has no row in
the table. What still isn't measured is *use* — a primitive can be reachable and
have no call site, and no build will say so.
