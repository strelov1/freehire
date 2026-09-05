## Why

The "Finish setting up your account" checklist only appears on `/my/tracking`, a page about tracked jobs — not about the account fields the checklist itself measures (role, skills, location). The fields it tracks are all edited on `/my/profile`, so the checklist should live where the work happens. On top of that, three of its five steps already point at `/my/profile` but always land on that page's default tab, so a visitor who clicks "List your skills" from the checklist still has to find the Skills tab themselves.

## What Changes

- Move the account-setup checklist card from `/my/tracking` to `/my/profile`, shown above the tab strip only once a profile exists (the pre-profile setup state already shows the create-profile form, which is itself the first step).
- Remove the checklist entirely from `/my/tracking` — the page keeps its heading and Board/List/Pipeline/Calendar tabs unchanged.
- Each outstanding step whose target is `/my/profile` (role, skills, location) now links to that field's own profile tab instead of the default tab.
- The profile page keeps its visible tab in sync with the `?tab=` query parameter for the lifetime of the page, not only at initial load — needed because the checklist and its links now live on the same page they navigate within, so a click may only change the query string without remounting the page.

## Capabilities

### New Capabilities
- `account-setup-checklist`: where the "finish setting up your account" checklist is shown, and that its outstanding steps deep-link into the specific profile tab where the missing field is edited.

### Modified Capabilities
(none — the checklist and the profile page's tab behavior were never captured as OpenSpec capabilities)

## Impact

- `web/src/lib/accountCompleteness.ts` — steps gain an optional target tab.
- `web/src/lib/components/AccountSetupCard.svelte` — links append the tab when the step names one.
- `web/src/routes/my/tracking/+layout.svelte` — checklist removed.
- `web/src/routes/my/profile/+page.svelte` — checklist added; tab selection becomes reactive to the URL.
- `web/src/lib/accountCompleteness.test.ts` — existing coverage must keep passing unchanged.
