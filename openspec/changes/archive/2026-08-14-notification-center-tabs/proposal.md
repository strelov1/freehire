## Why

Everything notification-related today lives in three unconnected places:
account-level settings at `/my/notifications`, the full delivery history at
`/my/notifications/history` (only linked from a sentence on the settings page),
and per-saved-search alert management at `/my/searches` (a separate sidebar
item, "Search notifications"). A user has no single place to see what they've
been sent and adjust everything that controls it.

## What Changes

- New tabbed section at `/my/notifications` with three real routes sharing one
  tab strip: `/my/notifications` (history, the new landing page),
  `/my/notifications/searches` (per-saved-search alerts), and
  `/my/notifications/settings` (the account-level reminder/nudge toggle +
  channels).
- **BREAKING** (URL only, redirected): `/my/searches` and
  `/my/notifications/history` 308-redirect to their new homes; no page content
  is lost.
- The sidebar's separate "Search notifications" entry is removed — the single
  "Notifications" entry now covers all three sub-routes (its existing
  prefix-match active-state rule already supports this).
- Pure reorganization: no new backend behavior, no new settings. Per-kind
  notification frequency/timing, a profile timezone field, and a new
  "profile match" notification kind are explicitly out of scope — separate,
  already-discussed follow-up projects.

## Capabilities

### New Capabilities
- `notification-center-navigation`: the shared tab strip and route structure
  for `/my/notifications` (history/searches/settings), including the
  redirects from the two retired URLs.

### Modified Capabilities
- `account-navigation`: the account section nav list drops the standalone
  "Search notifications" item (merged into "Notifications").
- `saved-searches`: the "Saved searches section in the account area"
  requirement moves from `/my/searches` to `/my/notifications/searches`.

## Impact

- **Web routes**: `web/src/routes/my/notifications/+layout.svelte` (new),
  `web/src/routes/my/notifications/+page.svelte` (content swaps from settings
  to history), `web/src/routes/my/notifications/settings/+page.svelte` (new,
  today's settings content), `web/src/routes/my/notifications/searches/+page.svelte`
  (new, today's `/my/searches` content), two new redirect `+page.ts` files.
  `web/src/routes/my/notifications/[id]/jobs/+page.svelte` is untouched.
- **Nav**: `web/src/lib/accountNav.ts`, `web/src/lib/accountNavIcons.ts`.
- No Go/backend changes, no migrations, no API changes.
