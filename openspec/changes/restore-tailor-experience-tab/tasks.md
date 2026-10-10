## 1. Restore the Experience tab and reseed-offer wiring

- [ ] 1.1 In `web/src/routes/tailor/[slug]/+page.svelte`, import `offerCvRefresh` +
      `TAILOR_REFRESH_MESSAGE` from `$lib/cvRefreshOffer`, `askCvRefresh` from
      `$lib/cvRefreshDialog.svelte`, and `ExperienceBankView` from
      `$lib/components/ExperienceBankView.svelte`.
- [ ] 1.2 Add `'experience'` to the `LeftTab` type union and to `leftTabs`, positioned
      between `'editor'` and `'templates'`.
- [ ] 1.3 Add `'experience'` to the `MobileView` type union and to `mobileTabs`, and to the
      `pickMobile` left-tab-sync condition.
- [ ] 1.4 Add `'experience'` to the mobile-visibility class list on the left `<section>`
      (the `mobileView === 'chat' || ...` condition).
- [ ] 1.5 Add `offerRefreshAfterBankEdit()`, calling today's `applyReseed`/`resetLocked`
      (not the pre-rename `applyResetFromResume` names), mirroring
      `/my/profile/experience/+page.svelte`'s existing use of the same
      `offerCvRefresh`/`askCvRefresh` pattern.
- [ ] 1.6 Add the Experience panel (`class:hidden={leftTab !== 'experience'}`) mounting
      `<ExperienceBankView onBankMutated={offerRefreshAfterBankEdit} />`, with the same
      `resetError` banner the History tab's reset failure already uses.

## 2. Verification

- [ ] 2.1 `pnpm check` in `web/` — error/warning count unchanged from baseline (no new
      issues introduced by this file).
- [ ] 2.2 `pnpm test` (vitest) in `web/` — full suite stays green.
- [ ] 2.3 Start the Vite dev server and confirm `/tailor/<slug>` compiles and
      server-renders without error (full authenticated manual check needs a running
      Postgres + API + logged-in session — out of reach in this worktree; note this
      limitation rather than claim a browser-verified Experience tab).
