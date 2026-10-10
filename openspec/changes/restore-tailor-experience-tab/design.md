## Context

The Experience tab and its bank-edit reseed offer existed in the Tailor workspace from
#1669 until `ae1c13c8` removed them. Nothing else in the file changed shape in a way that
blocks restoring them: `ExperienceBankView`'s `onBankMutated` prop, `offerCvRefresh`/
`askCvRefresh`, and the reseed endpoint (`api.reseedCv`, now wrapped by `applyReseed`/
`reseed`/`confirmResetOpen` under the History tab's "Reset" control) are all still present
and already used the same way by `/my/profile/experience/+page.svelte` for the base-CV
equivalent of this offer.

## Goals / Non-Goals

**Goals:**
- Re-add the `experience` left-panel tab, its mobile-tab-bar equivalent, and the
  bank-edit → reseed-offer wiring, restoring the behavior `ae1c13c8` removed.
- Reuse the existing `offerCvRefresh`/`askCvRefresh`/`ExperienceBankView` surfaces exactly
  as `/my/profile/experience` already does, so the two call sites stay in sync by
  construction rather than by convention.

**Non-Goals:**
- No change to how often a bank edit syncs into an *already-rendered* CV automatically —
  reseed stays an explicit, confirmed action (`ConfirmDialog`), not an auto-apply.
- No change to the assistant's Projects-vs-Experience filing behavior or the PII filter
  (separate issue-#2130 findings, tracked as later changes).
- No backend change.

## Decisions

- **Reuse over reinvent**: mount `ExperienceBankView` unmodified rather than building a
  Tailor-specific variant, since the component already fetches its own data and the
  `/my/profile` page already proves it works standalone.
- **Match current naming, not the pre-removal diff verbatim**: the removed commit's
  `applyResetFromResume`/`resetLocked`-adjacent names have since been renamed (the
  "reset-from-resume → reseed" rename); the restored `offerRefreshAfterBankEdit` calls
  today's `applyReseed`/`resetLocked`, not the old names, so the file doesn't regress the
  rename.

## Risks / Trade-offs

- [Risk] Spec drift: `tailor-workspace`'s "three-column surface" and "collapses on
  mobile" requirements were already out of sync with the code before this change (e.g.
  the mobile enumeration omits `letter`/`history`, added by later features without a spec
  update) — a pre-existing gap, not something this change caused.
  → Mitigation: keep this change's delta narrowly scoped to `experience` — add it to the
  mobile tab-bar enumeration (a precise list, so the addition is unambiguous) and add new,
  separate requirements for the desktop Experience tab and the reseed offer, rather than
  rewriting the already-imprecise three-column prose to fix unrelated drift.
- [Risk] A future refactor of the reseed control (History tab) could rename
  `applyReseed`/`resetLocked` again and silently break this second call site.
  → Mitigation: none beyond the existing test suite; both call sites already share the
  same `offerCvRefresh` helper, so a signature change fails type-checking at both sites.
