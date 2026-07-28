# Design system

`freehire-design-system` — tokens and Svelte primitives, a **pnpm** package sibling to
`web/`, linked in as `file:../design-system`. `web/src/lib/ui/index.ts` is a thin re-export
surface, so app code imports from `$lib/ui` and never reaches into the package directly.

Being extracted from `web/` in phases by an external contributor; the phase inventory lives
in `design-system/docs/`. Scripts that still `echo` a placeholder (`storybook`,
`validate:docs`) are unfinished phases, not breakage.

## Always true

- **pnpm, not npm.** `pnpm install && pnpm build` here, then `pnpm install` in `web/`.
  Corepack activates the pinned version. There is no `package-lock.json` anywhere — adding
  one breaks the CI `design-system` → `web` job chain.
- **A new utility class used inside `design-system/src/*.svelte` needs `web/src/app.css` to
  cover it.** The package resolves under `node_modules/`, which **Tailwind v4's automatic
  source detection ignores**, so its classes are dropped from web's built CSS unless
  `@source "../../design-system/src/**/*.svelte";` (already present, line 16) matches the
  file. Move a component into a path that line doesn't cover and its unique classes vanish.
- **Nothing in CI catches a dropped class.** Tailwind discards unknown classes silently, the
  build stays green, and the component just renders untinted. This is how
  `<Badge variant="missing">` lost its destructive tint in phase 4. Verify a moved or new
  primitive **visually**, not by a passing build.
- **Token custom properties are not automatically Tailwind utilities.** The z-index tokens
  ship from `dist/tokens-*.css` as plain `--z-modal` etc., but Tailwind mints `z-<name>` only
  from the `--z-index-*` namespace — hence the `@theme inline` aliases in `app.css`
  (`--z-index-modal: var(--z-modal);`). A new token family needs the same bridge or its
  utilities silently no-op.
- Tokens are authored as `tokens/*.tokens.json` and compiled by Style Dictionary
  (`scripts/build-tokens.mjs`) into `dist/`. Edit the JSON, run `pnpm build` — never hand-edit
  `dist/`.
- `svelte` and `tailwindcss` are **peer** dependencies; the package must not bundle its own
  copy.

## Limitations

- `web/pnpm-workspace.yaml` still carries unfilled `allowBuilds` placeholders
  (`'@sentry/cli': set this to true or false`). Harmless — `strictDepBuilds: false` governs
  the skip — but they should become real booleans.
- Storybook and the DSDS docs site are not configured yet.
