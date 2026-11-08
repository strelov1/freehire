## Why

freehire#2005 (item 1) lists `/my/notifications` among the still-untranslated
`/my/**` pages. Fifth PR of the fan-out, same pattern as the four merged
PRs before it.

## What Changes

- `web/src/routes/my/notifications/+page.svelte` gains a colocated
  `messages.ts` catalog (en/ru, plain — not `*.messages.ts` — since this
  page has no dedicated `$lib/components` view; the whole UI is the route
  file, matching `routes/my/profile/messages.ts`'s and
  `routes/my/security/messages.ts`'s precedent) and is migrated to render
  its three literal strings through it: `<title>`, the "Mark all read"
  button, and the "No notifications yet." empty-state message.
- **Out of scope**: `NotificationCard.svelte` (shared with the header bell
  on every route; has no visible literal text of its own beyond the
  already-locale-aware `timeAgo` call from the date-locale-formatting
  change — it does carry one screen-reader-only `aria-label="unread"`,
  pre-existing and untouched here, deferred to whenever that shared
  component itself gets translated) and the generic `$lib/ui` primitives
  this page uses (`LoadMore`, `States`) — same scope boundary the four
  earlier PRs in this series already establish.

## Capabilities

### New Capabilities

- `account-interface-i18n`: extends the same not-yet-synced capability the
  four earlier fan-out changes in this series declared.

### Modified Capabilities

(none)

## Impact

- 1 route file, 1 new catalog file.
- No backend, API, or database changes.
