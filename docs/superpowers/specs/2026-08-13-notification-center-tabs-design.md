# Notification center: one tabbed section instead of three scattered pages

## Problem

Everything notification-related today lives in three unconnected places: account-level
settings at `/my/notifications`, the full delivery history at `/my/notifications/history`
(only linked from a sentence on the settings page), and per-saved-search alert
management at `/my/searches` (a separate sidebar item, "Search notifications"). A user
has no single place to see what they've been sent and adjust everything that controls
it.

## Goal

One section, three real routes under a shared tab strip — not a client-side view
switch, actual navigable pages:

- `/my/notifications` — history (becomes the section's landing page)
- `/my/notifications/searches` — per-saved-search alerts (rename/share/delete + channel
  toggles, today's whole `/my/searches` page)
- `/my/notifications/settings` — the account-level reminder/nudge toggle + channels
  (today's whole `/my/notifications` page)

This is a pure reorganization. No new backend behavior, no new settings — those are
separate follow-up projects (notification frequency/timing + a profile timezone field;
profile-match notifications). Out of scope here.

## Routing

New `web/src/routes/my/notifications/+layout.svelte`: renders the design system
`TabStrip` (`$lib/ui`, same component already used on `/my/profile` and
`/my/market-pulse`) above `<slot />`. Unlike those two call sites, this strip is
route-driven, not local `$state`:

```ts
const TABS = [
  { id: 'history', label: 'History', href: '/my/notifications' },
  { id: 'searches', label: 'Search alerts', href: '/my/notifications/searches' },
  { id: 'settings', label: 'Settings', href: '/my/notifications/settings' },
] as const;

const active = $derived(
  page.url.pathname.startsWith('/my/notifications/searches')
    ? 'searches'
    : page.url.pathname.startsWith('/my/notifications/settings')
      ? 'settings'
      : 'history', // also the fallback for /my/notifications/[id]/jobs
);

function onSelect(id: (typeof TABS)[number]['id']) {
  void goto(resolve(TABS.find((t) => t.id === id).href));
}
```

The digest detail page `/my/notifications/[id]/jobs` (unrelated drill-down from a
history card, untouched by this change) falls under the shared layout too and lands on
the `history` tab highlighted — the closest match, since there's nowhere else for it to
belong.

## File moves

- `web/src/routes/my/notifications/history/+page.svelte`'s content moves to
  `web/src/routes/my/notifications/+page.svelte` (replacing today's settings content
  there).
- Today's `web/src/routes/my/notifications/+page.svelte` (renders `ReminderSettings` +
  the link to history) moves to `web/src/routes/my/notifications/settings/+page.svelte`,
  dropping the now-redundant "looking for what's been sent" link paragraph (the tab
  strip replaces it).
- `web/src/routes/my/searches/+page.svelte`'s content (`<SavedSearchesView />`) moves to
  `web/src/routes/my/notifications/searches/+page.svelte`.
- `web/src/routes/my/notifications/[id]/jobs/+page.svelte` — untouched, stays put.

## Redirects (old URLs keep working)

Three new `+page.ts` files, each a 308 (the existing convention — see `my/+page.ts`):

- `web/src/routes/my/notifications/history/+page.ts` → `/my/notifications`
- `web/src/routes/my/searches/+page.ts` → `/my/notifications/searches`

(Today's `/my/notifications` doesn't need a redirect file — it already resolves, to the
new history content, at the same path.)

## Nav

`web/src/lib/accountNav.ts`: drop the `{ href: '/my/searches', label: 'Search
notifications' }` entry. Keep the single `{ href: '/my/notifications', label:
'Notifications' }` entry — `isSectionActive`'s prefix match already highlights it for
all three sub-routes, no change needed there. `accountNavIcons.ts` loses the
`/my/searches` icon mapping (`Bell`); `/my/notifications` keeps `BellRing`.

## Testing

- `pnpm run check` (svelte-kit sync + svelte-check) and `pnpm test` on changed/new
  files.
- Manual: visit all three tabs, confirm the strip highlights correctly, confirm
  `/my/searches` and `/my/notifications/history` still resolve (redirect) to their new
  homes, confirm the digest detail page still opens from a history card with the
  history tab shown as active.

## Out of scope (future projects, already discussed and deferred)

- Per-notification-kind frequency (instant vs. daily digest vs. a specific time) and a
  user profile timezone field.
- A new "profile match" notification kind (CV-vs-vacancy match alerts) — a fourth
  engine alongside `notify`/`reminder`/`nudge`.
