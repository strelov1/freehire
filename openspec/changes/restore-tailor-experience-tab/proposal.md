## Why

GitHub issue #2130 reports that candidates have no way to manage work experience from
within the Tailor workspace, that added positions don't show up when tailoring a CV, and
that profile/assistant/tailoring data feels out of sync. The root cause: the Tailor
workspace's Experience tab (added in #1669, together with a prompt that offers to refresh
the tailored CV after a bank edit) was removed in commit `ae1c13c8`
("Remove the Experience tab from the CV tailoring workspace") with no replacement path. A
candidate must now leave Tailor for `/my/profile` to touch the bank, and nothing tells them
the tailored CV needs an explicit reseed afterward — so new positions silently never reach
the document being tailored.

## What Changes

- Restore the `experience` tab to the Tailor workspace's left panel (`Chat, Editor,
  Experience, Templates, Settings`), mounting the existing `ExperienceBankView.svelte`
  component unmodified.
- Restore the matching mobile-view entry and `pickMobile` wiring so the tab also works on
  narrow viewports.
- Restore `offerRefreshAfterBankEdit()`, wired as `ExperienceBankView`'s `onBankMutated`
  callback, so editing the bank from inside Tailor offers (via the existing
  `offerCvRefresh`/`askCvRefresh` dialog) to rebuild the tailored CV from the current seed —
  the same prompt already used on `/my/profile/experience`'s base-CV flow.
- No backend changes: `ExperienceBankView`, its API routes, the reseed endpoint, and
  `/my/profile`'s own usage of the component are untouched.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `tailor-workspace`: the left-sidebar tab set and the mobile tab bar/`pickMobile` switch
  regain the `experience` tab (between Editor and Templates), and gain a new requirement —
  not previously spec'd anywhere — that editing the bank from inside the workspace offers to
  reseed the tailored CV, mirroring the offer `/my/profile/experience` already makes for the
  base CV.

## Impact

- Frontend only: `web/src/routes/tailor/[slug]/+page.svelte` (new `LeftTab`/`MobileView`
  entries, `ExperienceBankView` panel, `offerRefreshAfterBankEdit` function, `pickMobile` and
  mobile-visibility class wiring).
- Unaffected: `internal/api/handler/me_experience.go`, `internal/candidate/experience/*`,
  `web/src/lib/components/ExperienceBankView.svelte`, `web/src/lib/cvRefreshOffer.ts`,
  `web/src/lib/cvRefreshDialog.svelte.ts`, and `/my/profile`'s own layout — all reused as-is.
- Out of scope (separate, later changes per issue #2130): the assistant occasionally filing
  work history under Projects instead of Experience, and the PII-filter interference with
  tailoring reported in the issue's comments.
