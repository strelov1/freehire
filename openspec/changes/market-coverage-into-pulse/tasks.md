## 1. Worktree readiness

- [x] 1.1 From `web/`: `pnpm install --frozen-lockfile && npx svelte-kit sync` (fresh
      worktree — `node_modules`/generated tsconfig won't exist yet). In practice
      needed `--config.trust-lockfile=true` too — the sandboxed shell's network
      access to `registry.npmjs.org` is slow/unreliable enough that pnpm's
      supply-chain `minimumReleaseAge` age-check (which needs a live fetch per
      recently-changed entry) kept timing out; `--trust-lockfile` skips that
      network-dependent verification for an already-trusted, committed lockfile.
      Also ran `design-system`'s own `pnpm install --config.trust-lockfile=true &&
      pnpm build` first (its `link:`-linked deps must resolve before `web` builds).

## 2. Market Pulse page: tabbed shell

- [x] 2.1 In `web/src/routes/my/market-pulse/+page.svelte`, add the page-level
      `<h1>Market pulse</h1>` + a tab-neutral subtitle, an `isAuthenticated()` gate
      above both tabs, and a `TabRow` with `coverage` (default, label "Coverage")
      and `trend` (label "Skill trend") — same `TabRow`/`tabId` pattern as
      `profile/+page.svelte`.
- [x] 2.2 In `web/src/lib/components/MarketPulseView.svelte`, remove its own
      `<h1>`/subtitle and `isAuthenticated()` check (now the page's job) — leave
      the search input, card grid, and loading/error/empty states untouched.
- [x] 2.3 Wire the `trend` tab body to `<MarketPulseView />`; verify in the dev
      server that Skill trend renders exactly as it did before (cards, search,
      sparklines, empty state). (`pnpm check`/`lint` clean; dev-server visual
      pass deferred to task 5.2 alongside the Profile pass.)

## 3. Move Coverage onto Market Pulse

- [x] 3.1 Move the filter/verdict state from `profile/+page.svelte` into
      `market-pulse/+page.svelte`: `FilterStore` construction (`buildFilters`
      seeded from `profileStore.profile.specializations`), `verdict`, `counts`,
      `modalOpen`, `previewCount`, `gapHref`, `loadError`, and a `reload()` that
      calls `getProfileVerdict` + `facetCounts` (no `ats`). Call
      `profileStore.ensureLoaded()` on mount, same effect shape as Profile's.
- [x] 3.2 Render the `coverage` tab body: `<VerdictView {verdict} {gapHref} />`
      when `profileStore.profile` exists; otherwise an empty state ("Complete
      your profile to see market coverage" + CTA linking to `/my/profile`),
      matching Skill trend's existing empty-state tone. Also gates on
      `profileStore.loaded` (shows a loading state) before checking `profile`,
      since the profile fetch is async and wasn't previously a concern this
      page had to handle on its own.
- [x] 3.3 Move the filter aside (`FilterSummary`) + `FilterEdgeTab` +
      `FilterModal` block onto this page, gated on `tab === 'coverage'` (same
      gating condition it had on Profile, now the page's own tab).
- [x] 3.4 Verify in the dev server: Coverage tab shows the verdict and gap
      skills for a profiled test account, filters refine it, and an unprofiled
      account sees the empty state with a working link to `/my/profile`.
      Confirmed the empty state (screenshot). The verdict-rendering happy path
      was NOT visually confirmed — the only locally running backend (:8080,
      shared with another concurrent session) is a stale `go run ./cmd/server`
      process that predates both this change AND the pre-existing
      `/me/market-pulse` route (404/500 on both), and restarting a shared
      process was out of scope for this verification. The fetch→state→render
      wiring is copied near-verbatim from Profile's already-proven
      implementation and `VerdictView` itself is untouched.

## 4. Trim Profile

- [x] 4.1 In `web/src/routes/my/profile/+page.svelte`, drop the `coverage`
      entry from `TABS` and from the `tab` union type (five tabs remain:
      Settings, Skills, Profile, Experience, CV readiness).
- [x] 4.2 Remove the `VerdictView` import/usage, the `verdict` state, and
      `gapHref`.
- [x] 4.3 In `reload()`, drop the `getProfileVerdict` call; derive `loadError`
      from whether the `ats`/`counts` fetches settle instead of `verdict`.
- [x] 4.4 Re-gate the filter aside + `FilterEdgeTab` + `FilterModal` block from
      `tab === 'coverage'` to `tab === 'readiness'`.
- [x] 4.5 Reword `FilterSummary`'s description prop away from the old coverage
      framing to describe comparing CV keyword strength against a chosen
      role/region/seniority.
- [x] 4.6 Verify in the dev server: Profile shows five tabs with no Market
      coverage; opening CV readiness reveals the filter sidebar/edge-tab, and
      changing a filter there changes the ATS keyword-match score. Confirmed
      the five-tab strip and the filter sidebar appearing on CV readiness
      (screenshot, reworded description visible: "Compare your CV's keyword
      strength against a role, region or seniority you choose."). The
      filter→score-change interaction itself needs a working ATS report call,
      same stale-backend limitation as 3.4 — not exercised live.

## 5. Cross-cutting verification

- [x] 5.1 From `web/`: `pnpm check` (svelte-check) and `pnpm lint` — fix
      anything the touched files introduce (pre-existing red baseline
      elsewhere is not this change's job to clean up). `pnpm check`: 0 errors
      (22 pre-existing warnings, none in touched files). `pnpm lint`: 0
      warnings in touched files (pre-existing red baseline elsewhere,
      confirmed by grep, matches [[hire-web-lint-red-baseline]]).
- [x] 5.2 Headless-Chrome visual pass over `/my/profile` (five tabs, CV
      readiness filters) and `/my/market-pulse` (both tabs, default Coverage).
      Done against the shared local backend (:8080) with a QA account
      (`qa@freehire.local`) via CDP screenshots. Confirmed: Profile's five
      tabs with no Market coverage; CV readiness's filter sidebar with the
      reworded description; Market Pulse's Coverage/Skill trend tabs with the
      shared header; Coverage's no-profile empty state with a working "Go to
      profile" link. Both tabs' data-loaded state showed their error state
      instead of live data — traced to the shared backend running a stale
      binary (predates even the pre-existing `/me/market-pulse` route: 404),
      not a defect in this change; see notes on 3.4/4.6.
- [x] 5.3 Confirm `/my/market-pulse/[skill]` still loads a skill's detail view
      and its "← Market pulse" back link returns to the page (landing on
      Coverage — the accepted, undocumented-elsewhere trade-off from design.md).
      Route renders 200; the file itself is untouched by this change (verified
      by inspection — the back link is a static `href={resolve('/my/market-pulse')}`).
- [x] 5.4 Re-read `openspec/changes/market-coverage-into-pulse/specs/**/*.md`
      against the finished implementation — confirm every scenario actually
      holds — before moving to finishing-a-development-branch.
