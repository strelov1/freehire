# Design system

`freehire-design-system` — tokens and Svelte primitives, a **pnpm** package sibling to
`web/`, linked in as `file:../design-system`. `web/src/lib/ui/index.ts` is a thin re-export
surface, so app code imports from `$lib/ui` and never reaches into the package directly.

Being extracted from `web/` in phases by an external contributor; the phase inventory lives
in `design-system/docs/`. A script that still `echo`s a placeholder (`validate:docs`) is an
unfinished phase, not breakage.

## Always true

- **pnpm, not npm.** `pnpm install && pnpm build` here, then `pnpm install` in `web/`.
  Corepack activates the pinned version. There is no `package-lock.json` anywhere — adding
  one breaks the CI `design-system` → `web` job chain.
- **`src/theme.css` is the package's CSS contract, and every consumer imports it.** It holds
  the token imports, the `@source` scan, and the `@theme inline` bridge. `web/src/app.css`
  and `.storybook/preview.css` each `@import` it and add only what is theirs (the typography
  plugin and app base layer; Tailwind's entry and the story-file scan). Put anything the
  primitives themselves depend on in `theme.css` — a second copy in a consumer is how the app
  and Storybook start disagreeing.
- **The `@source` scan is what keeps the primitives styled.** The package resolves under
  `node_modules/`, which **Tailwind v4's automatic source detection ignores**, so its classes
  are dropped from the built CSS unless `@source "./**/*.svelte"` in `theme.css` matches the
  file. `@source` paths resolve against the file holding them, so that one line covers every
  consumer — but move a component into a path it doesn't cover and its unique classes vanish.
  Story files are deliberately outside it: `.storybook/preview.css` scans them separately so
  the app never ships a utility only a story uses.
- **Tailwind drops unknown classes silently and the build stays green.** This is how
  `<Badge variant="missing">` lost its destructive tint in phase 4, and how phase 5's
  Storybook shipped with no Tailwind at all — 10 stories, a passing `build-storybook`, and
  every primitive rendering as unstyled markup. CI now asserts positively that
  `storybook-static` contains `.animate-pulse`, which only the scan over `src/*.svelte` can
  put there. That guards the wiring, not each class: still verify a moved or new primitive
  **visually**.
- **Token custom properties are not automatically Tailwind utilities.** The z-index tokens
  ship from `dist/tokens-*.css` as plain `--z-modal` etc., but Tailwind mints `z-<name>` only
  from the `--z-index-*` namespace — hence the `@theme inline` aliases in `theme.css`
  (`--z-index-modal: var(--z-modal);`). A new token family needs the same bridge or its
  utilities silently no-op.
- **Dark tokens live under a `.dark` selector**, so switching theme means putting a class on
  a root element, not swapping a stylesheet. Storybook does it with `addon-themes`'
  `withThemeByClassName` on `html`; a `backgrounds` toolbar option cannot — it only paints
  the canvas.
- Tokens are authored as `tokens/*.tokens.json` and compiled by Style Dictionary
  (`scripts/build-tokens.mjs`) into `dist/`. Edit the JSON, run `pnpm build` — never hand-edit
  `dist/`.
- `svelte` and `tailwindcss` are **peer** dependencies; the package must not bundle its own
  copy. Both are also devDependencies, because Storybook and the tests build against them
  here.

## Storybook

`pnpm storybook` (port 6006) / `pnpm build-storybook`. Stories are CSF in
`src/*.stories.ts`; `src/story-text.ts` wraps a plain string in the snippet Svelte 5 needs
for a component's `children`.

`docgen: false` in `.storybook/main.ts` — the svelte docgen plugin hands raw `.svelte` source
to the bundler's JS parser and dies on the markup. The cost is the autodocs prop tables, so
the Docs tab renders thin, and `argTypes` are hand-maintained: they drift from a component's
actual variants with nothing to catch it.

## Limitations

- Five primitives have no stories — `Dialog`, `FormField`, `Table`, `Tabs`, `Tooltip` — and
  neither do `Button`'s `destructive` variant and `icon` size, nor `Chip`'s `secondary`.
- The DSDS docs site is not configured yet.
