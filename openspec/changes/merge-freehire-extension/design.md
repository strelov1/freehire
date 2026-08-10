## Context

`../freehire-extension` is a separate git repo (51 commits, ~2 weeks old): WXT +
Svelte, npm-managed, with the actual package nested one level deeper than the
repo root (`freehire-extension/extension/`). Its sidepanel styles itself by hand
— scoped `<style>` blocks with literal hex colors (`#2563eb`, `#e5e5e5`, ...),
no tokens, no shared components, no Tailwind.

`freehire-design-system` (this repo, `design-system/`) is a Tailwind v4 +
Svelte 5 component/token package. `web/` consumes it via pnpm's
`"freehire-design-system": "link:../design-system"` and, critically, also pulls
in Tailwind itself — the package's `theme.css` is not standalone CSS, it
requires `@import "tailwindcss"` first (its `@theme`/`@custom-variant` blocks
are Tailwind directives) and depends on `@tailwindcss/vite` running in the
consumer's own Vite pipeline. WXT is Vite-based and accepts a `vite:` config
callback in `wxt.config.ts`, so the same wiring `web/vite.config.ts` uses is
available. The difference is npm vs. pnpm: npm has no `link:` protocol at all
(confirmed — `npm install` fails with `EUNSUPPORTEDPROTOCOL` on it); its
equivalent for a local directory dependency is `file:../design-system`, which
also installs as a symlink (confirmed via `ls -la node_modules/`), just under
a different specifier.

Neither `web/` nor `design-system/` carries its own `CLAUDE.md` — only the repo
root does, as a symlink to `AGENTS.md`. `extension/` follows the same
convention: `AGENTS.md` only, no symlink. The source repo's `CLAUDE.md` is a
byte-identical duplicate of its `AGENTS.md` and is dropped.

## Goals / Non-Goals

**Goals:**
- `extension/` lands in this repo as a normal top-level package, buildable and
  testable on its own, with no lingering double-nesting.
- The sidepanel renders through `freehire-design-system` components and tokens
  everywhere it currently hand-rolls CSS.
- Nothing behavioral changes: same agent, same tools, same auth flow, same
  autofill/match-card logic — only where the code lives and how it looks.
- The extension's own OpenSpec history (4 open changes, 3 specs) and AGENTS.md
  carry over instead of being lost.

**Non-Goals:**
- No pnpm/workspace migration for `extension/` — it keeps npm and
  `package-lock.json`.
- No change to `internal/assistant`, `internal/browsertools`, or the
  `extension-auth`/`extension-autofill` specs.
- No redesign of the sidepanel's information architecture (what's on screen,
  in what order) — this is a component/token swap, not a UX rework.
- No git history transplant (`git subtree`) — the old repo keeps its 51
  commits; this repo gets one fresh commit.

## Decisions

**1. Copy, don't subtree.** A plain copy (rsync-style, excluding
`node_modules/`, `.wxt/`, `.output/`, `.DS_Store`) into `extension/`, committed
once. Rationale: the repo is 2 weeks old and small; `git subtree add` would
work but adds merge-commit noise for history nobody will meaningfully `git
blame` through. The old repo is archived (read-only) on GitHub, so the history
isn't lost — just not interleaved into this repo's log.

**2. Flatten during the copy, not after.** Copy
`freehire-extension/extension/*` (the WXT project) directly to `hire/extension/`,
and `freehire-extension/{AGENTS.md,docs/,openspec/}` to their respective new
homes in one pass — never land the double-nested shape and then fix it in a
second commit.

**3. Tailwind wiring lives in `extension/`, not shared with `web/`.** Add
`tailwindcss`, `@tailwindcss/vite` as devDependencies of `extension/`, and a
`vite: () => ({ plugins: [tailwindcss()] })` entry in `extension/wxt.config.ts`
— mirroring `web/vite.config.ts`'s `tailwindcss()` call, not importing it.
New `extension/entrypoints/sidepanel/app.css`:
```css
@import "tailwindcss";
@import "freehire-design-system/theme.css";
```
imported once from `sidepanel/main.ts`, replacing the `:global(body)` reset
currently inlined in `App.svelte`'s `<style>` block.

**4. Restyle by direct component swap, not a wrapper layer.** Each hand-rolled
element maps to an existing `design-system/src/*` primitive:
- Sign-in / link buttons (`.signin`, `.link`) → `Button` (variants for
  primary vs. ghost).
- Status pill (`.status`, `.status.online`) → `Badge`.
- Auth error / chat error banners → `Alert`.
- Draft input → `Input` (wrapped in `FormField` where there's a label).
- Match card empty/loading states → `EmptyState` / `Skeleton`.
- Card chrome around `MatchCard`, `JobDeckCard` → `Card`.
No new design-system components are introduced by this change — if a
sidepanel need doesn't map to an existing primitive, it keeps local styling
for now rather than growing the design system as a side effect (that's a
separate, deliberate change).

**5. Docs and OpenSpec carry-over is a folder copy, not a rewrite.**
- `openspec/changes/{agent-autofill-wire,apply-form-readiness,combobox-actuation,local-harness-wire}`
  and the extension-specific entries under `openspec/changes/archive/` copy in
  unmodified (checked: no name collisions with this repo's existing changes).
- `openspec/specs/{browser-tool-page-read,panel-assistant-chat,panel-contribute-page}`
  copy in unmodified (checked: no collisions).
- `openspec/config.yaml` is dropped (byte-identical to this repo's).
- `.claude/skills/`, `.claude/commands/opsx/` are dropped (redundant with the
  superpowers/opsx plugins already available here).
- `AGENTS.md` → `extension/AGENTS.md`, trimmed of the "Working principles"
  section (already stated, near-verbatim, in this repo's root `AGENTS.md`) —
  keep the extension-specific "What this is" / stack / auth-flow content. Add
  one row to the root `AGENTS.md` module table pointing at it.
- `docs/chrome-web-store.md` → `extension/docs/chrome-web-store.md` unchanged
  — its instructions already say `extension/package.json`,
  `extension/wxt.config.ts`, which resolve correctly once flattened.

**6. CI as a new, independent workflow file.** `.github/workflows/extension.yml`,
adapted from the source repo's `build.yml`: same jobs (`npm ci`, `npm test`,
`npm run check`, `npm run zip`, upload the `chrome-mv3` artifact), gated with
`paths: ['extension/**', 'design-system/**', ...]` on both `push` and
`pull_request` so it doesn't run on every Go/`web/` change. Kept separate from
`ci.yml` rather than merged in, matching this repo's existing
one-file-per-concern layout (`ci.yml`/`govulncheck.yml`/`perf.yml`/`pr-welcome.yml`).

A workflow-run artifact expires after 90 days — fine for reviewing a PR's
build, wrong for "give me the build we shipped two months ago." So a `push` to
`main` additionally publishes a **GitHub Release**, tagged
`extension-v<package.json version>`, with the zip as a release asset — this
does not expire. Skipped on `pull_request` (a PR build is not a release yet)
and made idempotent (`gh release view` before `gh release create`, so a re-run
of the same commit, or a second commit that doesn't bump the version, does not
fail on an already-existing tag). This means bumping `extension/package.json`'s
`version` is what produces a new named release — the same discipline
`docs/chrome-web-store.md` already requires before a store upload.

**7. Archive the old GitHub repo after, not before.** Land the copy, get CI
green on the new location, then archive (Settings → Archive repository)
`freehire-extension` on GitHub. Archiving is reversible (a repo can be
unarchived) and keeps the 51-commit history visible read-only rather than
deleting it.

## Risks / Trade-offs

- **[Tailwind conflicts with WXT's content-script CSS isolation]** → Tailwind
  only needs to apply inside the sidepanel's own document
  (`sidepanel/index.html`), which is already a fully separate page/iframe from
  any content script injected into the host page — no shared stylesheet scope
  to worry about. Verify by loading the built extension and confirming the
  content script (if it renders anything visible) is unaffected.
- **[No automated visual regression coverage]** → The sidepanel has vitest unit
  tests for logic (`lib/**/*.test.ts`) but nothing visual. Each restyled
  component gets a manual load-the-extension-and-look pass (per this project's
  `hire-ui-reposition-verify-visually` practice) before the task is marked
  done, not just `npm run check` passing.
- **[`file:../design-system` breaks if either package moves again]** → Same
  fragility `web/` already accepts with `link:`; not new risk introduced by
  this change.
- **[Archiving the GitHub repo too early strands in-flight local branches]** →
  Mitigated by decision 7's ordering: archive only after the copy's CI is
  green, and only once no one has uncommitted work on the old checkout (ask
  before archiving, per this project's standing rule about hard-to-reverse
  actions on shared systems).

## Migration Plan

1. Copy `extension/` (flattened) + `AGENTS.md` + `docs/` into a new branch of
   this repo; `.gitignore` gains the source repo's `node_modules/ .output/
   .wxt/ *.zip` entries scoped under `extension/` (verify no duplicate/broader
   entry already covers them — this repo's root `.gitignore` already ignores
   `node_modules/` globally).
2. Wire `freehire-design-system` + Tailwind into `extension/wxt.config.ts` and
   `package.json`; confirm `npm install && npm run dev` still boots the
   sidepanel unstyled-but-working before touching any component.
3. Restyle component-by-component (§ Decisions #4), verifying each visually
   before moving to the next.
4. Carry over OpenSpec changes/specs, add `.github/workflows/extension.yml`,
   add the root `AGENTS.md` module-table row.
5. Land the PR; confirm `extension.yml` is green on the new location.
6. Archive `freehire-extension` on GitHub (with the user's go-ahead, per the
   standing rule on actions affecting shared/external systems).

**Rollback:** everything through step 5 is additive to this repo (a new
directory, a new workflow file, two doc edits) — revert the PR if needed, no
data/schema involved. Step 6 (archiving) is independently reversible
(unarchive) and is the only step touching something outside this repo.

## Open Questions

- None outstanding — the scope, layout, and component mapping above are
  sufficient to start `tasks.md`.
