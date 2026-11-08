## Why

freehire#2005 (item 1) lists `/my/webhook` among the still-untranslated
`/my/**` pages. It is one of the smallest — a single self-contained
component — and a natural next slice after `i18n-profile-small-cards`
(merged) proved the pattern again on account-settings cards.

## What Changes

- `web/src/lib/components/WebhookSettingsView.svelte` gains a colocated
  `WebhookSettingsView.messages.ts` catalog (en/ru) and is migrated to
  render through `t(messages, locale())`: the sign-in prompt, heading,
  description, form label/placeholder, button states (Create/Save/Saving…),
  the three form-error messages, the enabled/disabled status line (already
  locale-aware for its `timeAgo` calls since the date-locale-formatting
  change), the Enable/Disable/Delete buttons, and the delete-confirmation
  dialog's title/description/confirm label.
- `web/src/routes/my/webhook/+page.svelte`'s `<title>Webhook — freehire</title>`
  moves into the same catalog as a `headTitle` key, matching the pattern
  `MySubmissionsView`'s migration already established (page title rendered
  from the component's own catalog, not left hardcoded in the route).
- **Out of scope**: `States.svelte`, the shared loading/empty/error
  component this view renders — it is reachable from public routes too
  (`routes/l/[slug]/+page.svelte` among others), and in this specific view
  it is only ever invoked with `state="loading"`, which renders no text at
  all (skeleton rows only), so there is nothing of this page's own to
  translate there.

## Capabilities

### New Capabilities

- `account-interface-i18n`: extends the same not-yet-synced capability
  `i18n-profile-small-cards` and the earlier `i18n-my-account`/
  `i18n-my-account-fanout` changes declared (see those proposals for why
  this is "new" rather than "modified" — no spec file exists yet under
  `openspec/specs/` for it).

### Modified Capabilities

(none)

## Impact

- 1 component file, 1 new catalog file, 1 route file (title only).
- No backend, API, or database changes.
