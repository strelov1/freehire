# Design system

`freehire-design-system` — tokens and Svelte primitives, a **pnpm** package sibling to
`web/`, linked in as `file:../design-system`. `web/src/lib/ui/index.ts` is a thin re-export
surface, so app code imports from `$lib/ui` and never reaches into the package directly.

Being extracted from `web/` in phases by an external contributor; the phase inventory lives
in `design-system/docs/`. A script that still `echo`s a placeholder is an unfinished phase,
not breakage.

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
`src/*.stories.ts`, one file per primitive, and all 15 are covered.

**Two ways to give a story its `children`, and the choice is not taste.** For a primitive
whose children are just text, `src/story-text.ts` wraps a string in the snippet Svelte 5
requires. For one whose children compose *other primitives* or take snippet parameters —
`Dialog` needs a trigger, `FormField` hands its control `{ id, describedBy, required,
invalid }`, `Table` leaves rows to the caller, `Tooltip` wires `aria-describedby` onto the
first focusable child — a raw HTML string would mean hand-copying the markup of `Input` or
`Button`, and the story would drift from the component silently. Those get a real component
in **`.storybook/demos/`**, named in the story's `component`.

The demos live there and not in `src/` for two reasons: `src/*` is the package's public
export surface (`"./*": "./src/*"`), and `theme.css`'s `@source "./**/*.svelte"` would mint
their layout utilities into the *app's* bundle. `preview.css` scans them separately.
(`@storybook/addon-svelte-csf` would be the tidier answer, but 5.1.2 peers `@storybook/svelte`
at `^10.4.0-0` and we are on 10.5.4.)

`docgen: false` in `.storybook/main.ts` — the svelte docgen plugin hands raw `.svelte` source
to the bundler's JS parser and dies on the markup. The cost is the autodocs prop tables, so
the Docs tab renders thin, and `argTypes` are hand-maintained: they drift from a component's
actual variants with nothing to catch it. `Button`'s list already did, losing `destructive`.

## DSDS docs

`docs/dsds/*.json` describes the system as entities a docs site can render: `foundation.json`
(one entity per token family, each listing its token names), `theme.json` (the dark mode and
its `.dark` selector), `components.json` (one entity per primitive — props, defaults, and
links to its source and story).

`pnpm validate:docs` (`scripts/validate-docs.mjs`, run in CI) parses every file and checks
that each entity carries `id`, `type` and `name`. **That is structure only — nothing compares
an entity against the component or token it claims to describe.** A variant added to a `tv()`
call, a token added to `tokens/*.tokens.json`, or a story file added under `src/` leaves the
JSON stale and CI green, the same way hand-maintained `argTypes` drift. Touch a primitive or a
token family, update its entity in the same commit.

## Limitations

- The DSDS docs site itself is not configured yet — only the entity JSON it would consume.
