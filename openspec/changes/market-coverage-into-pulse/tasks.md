## 1. Worktree readiness

- [ ] 1.1 From `web/`: `pnpm install --frozen-lockfile && npx svelte-kit sync` (fresh
      worktree — `node_modules`/generated tsconfig won't exist yet).

## 2. Market Pulse page: tabbed shell

- [ ] 2.1 In `web/src/routes/my/market-pulse/+page.svelte`, add the page-level
      `<h1>Market pulse</h1>` + a tab-neutral subtitle, an `isAuthenticated()` gate
      above both tabs, and a `TabRow` with `coverage` (default, label "Coverage")
      and `trend` (label "Skill trend") — same `TabRow`/`tabId` pattern as
      `profile/+page.svelte`.
- [ ] 2.2 In `web/src/lib/components/MarketPulseView.svelte`, remove its own
      `<h1>`/subtitle and `isAuthenticated()` check (now the page's job) — leave
      the search input, card grid, and loading/error/empty states untouched.
- [ ] 2.3 Wire the `trend` tab body to `<MarketPulseView />`; verify in the dev
      server that Skill trend renders exactly as it did before (cards, search,
      sparklines, empty state).

## 3. Move Coverage onto Market Pulse

- [ ] 3.1 Move the filter/verdict state from `profile/+page.svelte` into
      `market-pulse/+page.svelte`: `FilterStore` construction (`buildFilters`
      seeded from `profileStore.profile.specializations`), `verdict`, `counts`,
      `modalOpen`, `previewCount`, `gapHref`, `loadError`, and a `reload()` that
      calls `getProfileVerdict` + `facetCounts` (no `ats`). Call
      `profileStore.ensureLoaded()` on mount, same effect shape as Profile's.
- [ ] 3.2 Render the `coverage` tab body: `<VerdictView {verdict} {gapHref} />`
      when `profileStore.profile` exists; otherwise an empty state ("Complete
      your profile to see market coverage" + CTA linking to `/my/profile`),
      matching Skill trend's existing empty-state tone.
- [ ] 3.3 Move the filter aside (`FilterSummary`) + `FilterEdgeTab` +
      `FilterModal` block onto this page, gated on `tab === 'coverage'` (same
      gating condition it had on Profile, now the page's own tab).
- [ ] 3.4 Verify in the dev server: Coverage tab shows the verdict and gap
      skills for a profiled test account, filters refine it, and an unprofiled
      account sees the empty state with a working link to `/my/profile`.

## 4. Trim Profile

- [ ] 4.1 In `web/src/routes/my/profile/+page.svelte`, drop the `coverage`
      entry from `TABS` and from the `tab` union type (five tabs remain:
      Settings, Skills, Profile, Experience, CV readiness).
- [ ] 4.2 Remove the `VerdictView` import/usage, the `verdict` state, and
      `gapHref`.
- [ ] 4.3 In `reload()`, drop the `getProfileVerdict` call; derive `loadError`
      from whether the `ats`/`counts` fetches settle instead of `verdict`.
- [ ] 4.4 Re-gate the filter aside + `FilterEdgeTab` + `FilterModal` block from
      `tab === 'coverage'` to `tab === 'readiness'`.
- [ ] 4.5 Reword `FilterSummary`'s description prop away from the old coverage
      framing to describe comparing CV keyword strength against a chosen
      role/region/seniority.
- [ ] 4.6 Verify in the dev server: Profile shows five tabs with no Market
      coverage; opening CV readiness reveals the filter sidebar/edge-tab, and
      changing a filter there changes the ATS keyword-match score.

## 5. Cross-cutting verification

- [ ] 5.1 From `web/`: `pnpm check` (svelte-check) and `pnpm lint` — fix
      anything the touched files introduce (pre-existing red baseline
      elsewhere is not this change's job to clean up).
- [ ] 5.2 Headless-Chrome visual pass over `/my/profile` (five tabs, CV
      readiness filters) and `/my/market-pulse` (both tabs, default Coverage).
- [ ] 5.3 Confirm `/my/market-pulse/[skill]` still loads a skill's detail view
      and its "← Market pulse" back link returns to the page (landing on
      Coverage — the accepted, undocumented-elsewhere trade-off from design.md).
- [ ] 5.4 Re-read `openspec/changes/market-coverage-into-pulse/specs/**/*.md`
      against the finished implementation — confirm every scenario actually
      holds — before moving to finishing-a-development-branch.
