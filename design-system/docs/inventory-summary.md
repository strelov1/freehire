# Phase 1 investigation summary

Written inventory of primitives, tokens, and infrastructure touch points. Produced before any design-system code is written. Feeds phases 02 (package boundary), 03 (tokens), and 04 (primitives).

Refined post-phase-1: `.svelte` files are now classified into four buckets — **components** (reusable UI building blocks), **views** (page-level compositions), **layouts** (structural shells), and **patterns** (recurring multi-component compositions). Only **components** are the focus of the design-system primitive layer; the other three are deferred to Storybook (phase 05).

## Stack

- **Framework:** SvelteKit 2.66 + Svelte 5.56 (runes: `$props()`, `$state()`, `$bindable`, `$derived`).
- **CSS:** Tailwind v4 (`@import "tailwindcss"` + `@theme inline` in `web/src/app.css`), `tailwind-variants` 3.2 for component variants, `tailwind-merge` + `clsx` via `$lib/utils/cn`.
- **Package manager:** npm (`web/package-lock.json`). Phase 02 migrates to pnpm.
- **Node:** 22 (Dockerfile + CI).
- **Linting:** oxlint + eslint (flat config, syntactic only — type-aware linting left to svelte-check).
- **TypeScript:** strict, `noUncheckedIndexedAccess`, `noImplicitOverride`, `noImplicitReturns`.
- **Icon library:** `@lucide/svelte` 1.24 (single icon set, 172 usages).
- **Build:** Vite 8 + `@sveltejs/vite-plugin-svelte` 7 + `@sveltejs/adapter-node` 5.

## Existing primitives (`web/src/lib/ui/`)

4 files. All use Svelte 5 runes + `tailwind-variants`. These migrate into `design-system/src/` in phase 04.

| primitive | variants | notes |
|---|---|---|
| `button.svelte` | variant: primary/secondary/outline/ghost; size: sm/md/lg/icon | Uses `tv` with `buttonVariants`. Focus ring via `ring-ring`. |
| `badge.svelte` | variant: secondary/outline/brand | Uses `tv` with `badgeVariants`. |
| `input.svelte` | none (styled `<input>`) | `$bindable` value, `class` pass-through via `cn()`. Focus ring via `ring-ring/50`. |
| `skeleton.svelte` | none | Pure `animate-pulse rounded-md bg-muted` div. |

## Token gaps vs. `web/src/app.css`

### What exists today (port these first in phase 03)

All in `:root` (light) + `.dark` blocks. Color values are `oklch()` except brand family which is hex in light / oklch in dark.

- **Semantic colors:** background, foreground, card, card-foreground, popover, popover-foreground, primary, primary-foreground, secondary, secondary-foreground, muted, muted-foreground, accent, accent-foreground, destructive, border, input, ring.
- **Brand family ("oats green" — Granola palette):** brand, brand-foreground, brand-strong, brand-muted, brand-ring.
- **Radius:** `--radius` (0.625rem) with sm/md/lg/xl derivations in `@theme inline` (`calc(var(--radius) * N)`).
- **Fonts:** `--font-sans`, `--font-mono` in `@theme inline` (system font stacks).
- **Shiki code highlighting:** `--shiki-light` / `--shiki-dark` (consumed in `.docs-shiki` block, not a design token per se — a render hook).

### What's missing (create in phase 03)

- **Type scale:** no `--font-size-*` / `--font-weight-*` / `--line-height-*` tokens. Sizes come from Tailwind defaults (`text-sm`, `text-base`, etc.). Phase 03 should define a scale: `typography/size/{xs,sm,base,lg,xl,2xl,...}`, `typography/weight/{normal,medium,semibold,bold}`, `typography/line-height/{tight,normal,relaxed}`.
- **Spacing scale:** no `--spacing-*` tokens. Tailwind's default spacing scale is used implicitly. Phase 03 should define `spacing/{0,1,2,3,4,6,8,12,16,24,...}`.
- **Shadow / elevation:** no `--shadow-*` tokens. Any shadows are inline Tailwind utilities. Phase 03 should define `shadow/{sm,md,lg,xl}`.
- **Motion / duration / easing:** no `--duration-*` / `--ease-*` tokens. Transitions use inline `transition-colors`. Phase 03 should define `motion/duration/{fast,normal,slow}`, `motion/easing/{ease-in,ease-out,ease-in-out}`.
- **Z-index:** no `--z-*` tokens. Any z-index values are inline. Phase 03 should define `z-index/{base,dropdown,sticky,modal,popover,toast,tooltip}`.

### Token coverage rule (enforced continuously)

Every color, font-size/weight/line-height/family, and spacing value in component markup must resolve to a token. No hex/rgb/oklch literals, no Tailwind arbitrary values (`bg-[#...]`, `text-[13px]`, `p-[7px]`). Same for elevation/shadow, motion/duration, and z-index once those token groups exist. Every token gets both light and dark values at creation time.

## Infrastructure touch points (phase 02 changes all of these)

Read in full this phase. Listing exact current state so phase 02 knows what to change.

### `.github/workflows/ci.yml`

Two jobs: `backend` (Go) and `web` (Node). The `web` job:
- `actions/setup-node@v6` with `cache: npm` + `cache-dependency-path: web/package-lock.json`.
- `npm ci` → `npm run check` → `npm run build`.
- Working directory: `web`.

Phase 02 replaces with two pnpm jobs: `design-system` (build + test + tokens:build + validate:docs + build-storybook) and `web` (install + check + build, `needs: [design-system]`). Both use `pnpm/action-setup` + `actions/setup-node` with `cache: pnpm`.

### `web/Dockerfile`

Build context: `web/` only (`docker build -f web/Dockerfile web/`). Stages:
- **build:** `node:22-alpine`, `COPY package.json package-lock.json`, `npm ci`, `COPY . .`, `npm run build`, `npm prune --omit=dev`. Sentry source-map upload gated on build args.
- **runtime:** `node:22-alpine` + nginx, copies `build/`, `node_modules/`, `package.json`, `public/install.sh`, `nginx.conf`, `docker-entrypoint.sh`.

Phase 02 changes: corepack→pnpm, build context → repo root (sibling `file:` dependency unreachable from `web/`-only context), add a stage building `design-system` before `web`, `COPY` both dirs.

### `docker-compose.yml`

`app` service `build: .` (backend). No `web` service in the compose file (may be built separately or not dockerized locally). Phase 02 updates any `web` build context to match the new Dockerfile context.

### `CONTRIBUTING.md`

Lines ~88-89 under "For the frontend (`web/`)" reference `npm run check` / `npm run build`. Phase 02 updates to pnpm and adds the `design-system` build step.

### Root `AGENTS.md`

`## Layout` tree and module-file table. Phase 02 adds `design-system/` to both, pointing at `design-system/AGENTS.md` (written in phase 06).

## Decisions made this phase

- **`$lib/ui` as thin re-export** (low-churn default, confirmed for phase 04): `web/src/lib/ui/` keeps its barrel but re-exports from `freehire-design-system` instead of owning the primitives. ~90 import sites unchanged.
- **Single icon set:** `@lucide/svelte` stays the only icon library. Bespoke SVGs (brand marks, data-driven charts) stay bespoke and are logged in the icon inventory. Four replaceable inline glyphs flagged for cleanup.
- **`design-system/docs/` created in phase 01** (not phase 02): the inventory deliverables need somewhere to live, so this directory is created now. Phase 02 scaffolds the rest of `design-system/` (`package.json`, `src/`, `tokens/`).
