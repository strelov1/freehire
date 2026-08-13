# Design system

`freehire-design-system` — tokens and Svelte primitives, a **pnpm** package sibling to
`web/`, linked in as `link:../design-system`. `web/src/lib/ui/index.ts` is a thin re-export
surface, so app code imports from `$lib/ui` and never reaches into the package directly.
It re-exports with `export *`, not a list: an enumerated list is a second copy of `src/index.ts`
that nothing reconciles, and it drifted once already — eleven of the fifteen primitives were
built, tested, storybooked and documented while being unreachable from the app, with every
check green. `cn` comes from here too; `$lib/utils` no longer carries a copy.

Extracted from `web/` over seven phases, all landed. `docs/` holds what outlived them:
`verification.md` (each guarantee and the command that enforces it), `icons.md`, and the
DSDS entity JSON. The phase inventories were plans for finished work and are in git history
rather than here — their census of `web/` was already off by a third.

## Always true

- **pnpm, not npm.** `pnpm install && pnpm build` here, then `pnpm install` in `web/`.
  Corepack activates the pinned version. There is no `package-lock.json` anywhere — adding
  one breaks the CI `design-system` → `web` job chain.
- **`link:`, not `file:` — and the order matters.** `link:` is a bare symlink, so there is no
  copy of this package to go stale; editing a token here is visible to `web` on its next build
  with no install at all. It was `file:` until 2026-07-31, which copies into web's virtual
  store keyed by name+version — and this package's version is a permanent `0.0.0`, so
  `--frozen-lockfile` happily reused a copy from days earlier. That shipped an app referencing
  `bg-warning` against a design system defining no such token: green release, colourless
  badges. The price of the symlink is that **pnpm does not install a linked package's own
  dependencies** — `tailwind-variants`, `clsx`, `tailwind-merge` and `@lucide/svelte` resolve
  from `design-system/node_modules`, so **install here before building `web/`** or the build
  dies with `MODULE_NOT_FOUND`. Loud, which is the trade.
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
- **`.dark` carries only what dark changes.** The dark build reads `color-dark.tokens.json`
  alone, so `dist/tokens-dark.css` is 28 declarations, not a second copy of the whole scale —
  the cascade already inherits the rest from `:root`. A dark override of a non-colour token
  would need its file added to `darkSources`; nothing needs one yet.
- **A token name may not be authored twice.** `build-tokens.mjs` throws and names both files.
  Style Dictionary reports its own collisions as a bare count with the details behind a
  verbosity flag, so a real one reads as the number going up by one. The count that remains
  (8) is the inert kind: each token file's root `$description` and `$type` are the root
  group's metadata, they overwrite each other on merge, and nothing consumes them.
- **A primitive may not style itself outside the token scale.** `pnpm check:tokens` (in CI)
  fails on a colour literal or a Tailwind arbitrary value in `src/*.svelte` — both are a value
  the theme cannot move and `.dark` cannot override, and both compile fine, so nothing else
  would catch one. Arbitrary *variants* pass: `[&_tr]:border-b` is a selector, not a value.
  A handful of named exceptions are carried in the script's own `ALLOWED` list — each pinned
  to one file and one detector kind, with the reason it exists — and the script fails if any
  of them ever stops applying (an unused exception is exactly as wrong as an unlisted
  violation). See `docs/verification.md`.
- **The same check has a second radius over `../web/src`, and it is not the same rule.** The
  package is held at zero (modulo the `ALLOWED` list above); web is held at its current count
  *per file* in `scripts/web-token-baseline.json` (455 across 109 files), because a rule
  nobody can satisfy
  gets switched off rather than obeyed. Web also gets a third detector the package does not:
  `text-amber-600` is a well-formed utility off Tailwind's own palette — neither a literal nor
  an arbitrary value, invisible to both other detectors, and the majority of what web has.
  **Add a detector once, in `DETECTORS`, and put it in a radius** — never fork it.
- **Both baselines are exact in both directions.** An improvement is red too, and says so:
  rerun with `--update` and commit the diff. Do not "fix" that by loosening the comparison to
  `>=`. A ratchet that absorbs improvements silently sits at 459 while reality is 40, and the
  regression back to 459 passes green — asserting nothing.
- **`pnpm check:adoption` is the only check that reads both halves of the repo.** This job
  proves the package builds and the `web` job proves the app builds, and both are true whether
  or not they are connected — which is how eleven primitives stayed unreachable from app code
  for three phases with everything green. It counts the `web/src` files importing each
  primitive against `scripts/adoption-baseline.json`, and names the unused ones every run
  (nine of fifteen today). The primitive list is derived from `src/index.ts`, so a new
  primitive joins the census by itself — **do not list them in the script.**
- **`$lib/ui` is a wall, not a convention.** A `.svelte` or `.ts` file under `web/src` that
  imports `freehire-design-system` by name fails `check:adoption` outright — no baseline, no
  exception, because there are zero today. `web/src/app.css`'s `@import` of `theme.css` is
  exempt and must stay so: it is the CSS contract. The walk reads `from`-clauses only.
- **`dist/` is committed, so CI rebuilds and diffs it** (`pnpm check:dist`). Editing a token
  without running `pnpm build` used to ship the old CSS with every job green. Style Dictionary
  writes no timestamp, so the rebuild is byte-identical and the diff means something.
- **Both cross-boundary checks are repo checks that happen to live here.** They read a
  directory the package knows nothing about. If `design-system/` is ever extracted, they stay
  with the repo — do not treat them as part of the package's surface.
- **`pnpm test` runs two vitest projects.** `components` (`src/**/*.test.ts`, jsdom, the Svelte
  plugin, `vitest.setup.ts`) and `scripts` (`scripts/**/*.test.mjs`, plain node). A script test
  put under `src/` would pull in twenty seconds of Svelte transform to test a file walk; a
  component test under `scripts/` would get no DOM. Each script guards `main()` behind an
  `import.meta.url` check so importing it in a test does not run it.
- `svelte` and `tailwindcss` are **peer** dependencies; the package must not bundle its own
  copy. Both are also devDependencies, because Storybook and the tests build against them
  here.

## What `Dialog` does not cover

Ten surfaces in `web/` were built by hand before this primitive was reached for. Four are
now `Dialog` call sites — `AuthDialog`, `ReportDialog`, `RequestReferralModal`,
`DeleteAccountButton`. The other six are not stragglers; they are two gaps, and forcing
either onto `Dialog` means re-adding by hand what it deliberately does not do.

- **A sheet.** `JobDrawer` is a full-height drawer, `FilterModalShell` and
  `OnboardingWizard` stretch on mobile and centre above `sm`, `CookieConsent` is a bottom
  banner. `Dialog` is a centred modal and offers no positioning. Three of the four want the
  same thing — a **Sheet** primitive the system does not have.
- **A structured dialog.** `GmailConnectDialog` and `FollowUpDialog` both want a bordered
  header with an icon, a scrolling body and a bordered footer. `Dialog` renders its own
  `p-6` box with the title, the description and the close button in fixed positions, and
  `class` lands on `<dialog>` — it cannot be reshaped from the call site. Two call sites is
  the evidence; a `header`/`footer` snippet pair is the likely answer.

`CookieConsent` carries `role="dialog"` on a surface that is not modal — an accessibility
defect, listed here so it is not mistaken for a migration target.

**`dismissible={false}` holds a dialog open** while an irreversible request is in flight, so
Escape cannot hide whether it succeeded. Escape, the backdrop and the close button go away
together — leaving one is a dialog that claims to be held and is not.

**Preventing `cancel` is necessary and not sufficient**, and jsdom cannot show you why. The
platform's close watcher spends a user-activation budget on each `preventDefault`, and once it
runs out it fires `cancel` unprevented and closes anyway — measured in Chrome: two Escapes
refused, the third gets through. That valve exists so a page cannot trap someone. So a held
dialog also **reasserts itself in `onclose`**. Keep the window short and always leave the
caller a way out once the request resolves; a unit test that only asserts `defaultPrevented`
will pass either way.

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

`pnpm validate:docs` (`scripts/validate-docs.mjs`, run in CI) checks structure — every entity
carries `id`, `type`, `name` — and then the two things that actually rot: every `source.file`
and `stories[].file` resolves, a foundation entity's `tokens` list matches the keys of the
token file it names **in both directions**, and no `src/*.stories.ts` goes unclaimed. Phase 6
shipped with all three broken (every foundation path off by a directory, `destructive-foreground`
undocumented, five story files orphaned) and the structural check passed, which is why they are
checked now.

**What is still hand-maintained: `props`, their `values`, and every `description`.** A variant
added to a `tv()` call leaves the entity stale with CI green — the same drift that costs
`argTypes` their accuracy, and for the same reason: the values live inside a Svelte module
script that only a compiler can read. Touch a primitive, update its entity in the same commit.

## Limitations

- The DSDS docs site itself is not configured yet — only the entity JSON it would consume.
