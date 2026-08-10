## 1. Copy the extension in

- [x] 1.1 Copy `freehire-extension/extension/*` into a new top-level `extension/` in this repo (flattened — no double nesting), excluding `node_modules/`, `.wxt/`, `.output/`, `.DS_Store`.
- [x] 1.2 Add `extension/`'s ignore needs (`node_modules/ .output/ .wxt/ *.zip`) to this repo's `.gitignore`, scoped/verified against what's already covered by the existing global `node_modules/` entry.
- [x] 1.3 `cd extension && npm install && npm run dev` — confirm the sidepanel loads unstyled-but-functional (sign-in, chat) before touching any styling. (Verified via `npm install` + `npx wxt build`, a clean production build; the OAuth sign-in/chat flow itself needs a live Chrome load, done in task 3.1's visual pass instead of here.)
- [x] 1.4 `npm test && npm run check` inside `extension/` — confirm the carried-over vitest suite and svelte-check pass unmodified. (205/205 tests pass, 0 svelte-check errors.)

## 2. Wire in freehire-design-system

- [x] 2.1 Add `"freehire-design-system": "file:../design-system"` (corrected from the design's original `link:` — npm has no `link:` protocol; `file:` is npm's local-symlink equivalent, confirmed via `ls -la node_modules/`), `tailwindcss`, `@tailwindcss/vite` to `extension/package.json`.
- [x] 2.2 Add the `vite: () => ({ plugins: [tailwindcss()] })` entry to `extension/wxt.config.ts`, mirroring `web/vite.config.ts`.
- [x] 2.3 Create `extension/entrypoints/sidepanel/app.css` (`@import "tailwindcss";` then `@import "freehire-design-system/theme.css";`, plus a `@layer base` `body` rule mirroring `web/app.css`'s `bg-background text-foreground font-sans antialiased`) and import it from `sidepanel/main.ts`.
- [x] 2.4 Remove the `:global(body)` reset from `App.svelte`'s `<style>` block now that `app.css` owns it; confirmed via `npx wxt build` (clean build, CSS output grew from 6.1kB to 29.6kB — tokens/Tailwind are landing).

## 3. Restyle the sidepanel

- [x] 3.1 `App.svelte`: swapped `.signin` for `Button` (primary), `.status` for `Badge`, auth/chat error banners for `Alert`; composer buttons/input for `Button`/`Input`; remaining hand-rolled colors (message bubbles, `.who`, `.link`, borders) moved to `var(--token)` — no full primitive match exists for a chat bubble, so per design.md decision 4 these keep local styling, just tokenized instead of hex. **Not visually verified in a real browser** — this harness has no way to load an unpacked extension into Chrome and drive its OAuth sign-in; verified instead via `npx wxt build` (clean) + code-level review of the class/token mapping. Needs a manual look in Chrome before merging.
- [x] 3.2 `MatchCard.svelte`: wrapped in `Card`; the loading/error/empty states actually live in `App.svelte` (not `MatchCard` itself, which only renders once a match is `ready`) — swapped those for `Skeleton`/`EmptyState` there. Same visual-verification caveat as 3.1.
- [x] 3.3 `JobDeck.svelte` and `JobDeckCard.svelte`: swapped card chrome for `Card`, loading placeholders for `Skeleton`, remaining colors tokenized. Same caveat.
- [x] 3.4 `ToolGroupList.svelte`: no design-system primitive matches a `<details>` disclosure list (none exists in the package) — kept local styling per design.md decision 4, tokenized its colors. Same caveat.
- [x] 3.5 Grep `extension/entrypoints` and `extension/lib` for leftover literal hex colors (`#[0-9a-fA-F]{3,6}`) outside `design-system/` — none remain (only false-positive matches on Svelte `{#each ... (i)}` keys).
- [x] 3.6 `npm run check` — clean, 0 errors after the restyle (also `npm test` — 205/205 still passing).

## 4. Carry over docs and OpenSpec history

- [x] 4.1 Copy `freehire-extension/AGENTS.md` to `extension/AGENTS.md`, trimmed of the "Working principles" section (already covered by this repo's root `AGENTS.md`); added a note on the `design-system` styling wiring and the `pnpm install` prerequisite discovered while doing task group 2.
- [x] 4.2 Added a row for `extension/AGENTS.md` to the root `AGENTS.md` module table, plus a `Layout` bullet (mirroring the existing `design-system/` bullet) noting `extension/` is npm-managed and needs `design-system/` installed first.
- [x] 4.3 Copy `freehire-extension/docs/chrome-web-store.md` to `extension/docs/chrome-web-store.md` unchanged.
- [x] 4.4 Copy `freehire-extension/openspec/changes/{agent-autofill-wire,apply-form-readiness,combobox-actuation,local-harness-wire}` into this repo's `openspec/changes/`.
- [x] 4.5 Copy the extension-specific entries under `freehire-extension/openspec/changes/archive/` (`2026-07-25-panel-contribute-page`, `2026-07-28-page-read-boundary`, `2026-07-28-panel-on-freehire-assistant`) into this repo's `openspec/changes/archive/`.
- [x] 4.6 Copy `freehire-extension/openspec/specs/{browser-tool-page-read,panel-assistant-chat,panel-contribute-page}` into this repo's `openspec/specs/`.
- [x] 4.7 Re-verified at copy time (not just at design time): `ls openspec/changes/` and `ls openspec/specs/` before each copy — no collisions with any of the 4 changes or 3 specs.

## 5. CI

- [x] 5.1 Added `.github/workflows/extension.yml`, adapted from `freehire-extension/.github/workflows/build.yml` (upgraded to this repo's current action versions — `checkout@v7`/`setup-node@v7`, matching `ci.yml`), with `paths: ['extension/**', 'design-system/**', ...]` on `push`/`pull_request`, plus a `pnpm install` step for `design-system/`'s own deps (the same gotcha task 2.1/4.1 hit locally — mirrors `ci.yml`'s `web` job's "Install design-system dependencies" step almost verbatim).
- [x] 5.3 Added a "Publish GitHub Release" step to `extension.yml` (push-to-main only, idempotent, tagged `extension-v<package.json version>`) so a build survives past the 90-day workflow-artifact expiry — requested after the initial PR was opened.
- [x] 5.2 Pushed and opened PR #1726. `extension.yml` triggered correctly (confirmed via `gh run list --branch merge-freehire-extension` — only `extension`/`CI`/`govulncheck`/`Performance`/`PR Welcome` ran, consistent with the path filter) and passed in 45s: all steps green (design-system install, npm ci, tests, check, build+zip, artifact upload).

## 6. Land and retire the old repo

- [ ] 6.1 Open and merge the PR into `main`. PR #1726 opened: https://github.com/strelov1/freehire/pull/1726 — merge still pending on CI + the manual Chrome check.
- [ ] 6.2 Confirm with the user, then archive (not delete) `freehire-extension` on GitHub.
