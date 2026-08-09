## 1. One-click confirm on `ExperienceBankView`

- [x] 1.1 In `web/src/lib/components/ExperienceBankView.svelte`, add a `confirmAtom` function that
      calls `api.updateExperienceAtom(atom.id, { ...atom, claim: atom.claim })` (mirroring
      `saveEdit`'s call shape, no draft/textarea involved) and reloads the bank on success.
- [x] 1.2 Render a Confirm button (reuse the `Check` icon already imported) beside the existing
      Edit/Remove buttons, visible only when `unconfirmed(atom)` is true and the row is NOT
      currently `editing` — so it never appears alongside the edit-mode save button that reuses the
      same icon.
- [x] 1.3 Confirm the copy in the "not confirmed" banner still reads correctly now that there are
      two ways to resolve an unconfirmed atom (edit or confirm) — adjust its wording only if it
      still says "edit one to make it yours" as the sole option.

## 2. Experience tab in the Tailor workspace

- [x] 2.1 In `web/src/routes/tailor/[slug]/+page.svelte`, add `'experience'` to the `LeftTab` type
      union and to the `leftTabs` array, in the order `Chat, Editor, Experience, Templates,
      Settings` (reordering the existing array, not just appending).
- [x] 2.2 Add the new panel: `<div class="h-full overflow-auto p-4" class:hidden={leftTab !==
      'experience'}><ExperienceBankView /></div>`, alongside the existing Editor/Templates/Settings
      panels — import the component.
- [x] 2.3 Add `'experience'` to the mobile flat tab bar (`mobileTabs`) and to `pickMobile`'s
      column-sync branch (`v === 'chat' || v === 'editor' || ...`) so tapping it on mobile also
      selects the matching left-panel tab, matching how Editor/Templates/Settings already behave.

## 3. Verification

- [x] 3.1 `pnpm check` in `web/` — 0 new type errors.
- [x] 3.2 Live browser check (this repo has no Svelte component test harness): open `/tailor/[slug]`
      for an existing tailored CV, confirm the Experience tab appears in the right position, shows
      the same content as `/my/profile`, and the Confirm button on an `agent_inferred` atom (seed
      one via the assistant if none exists) re-stamps it to `manual` without opening the edit
      field — verify against a real Postgres/API, not just visually.
- [x] 3.3 Confirm `/my/profile`'s `ExperienceBankView` also shows the new Confirm button (shared
      component) and behaves identically there.
- [x] 3.4 Confirm the mobile tab bar (narrow viewport) offers Experience and switching to it also
      selects the desktop left-panel tab underneath.
