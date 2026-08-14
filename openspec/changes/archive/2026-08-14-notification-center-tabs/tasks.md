## 1. Audit

- [x] 1.1 Grep the whole `web/src` tree for literal `/my/searches` and `/my/notifications/history` string references (not just `resolve()` route ids) — update any found outside the files this change already touches (e.g. `ReminderSettings.svelte`'s "search notifications" link, `SavedSearchesView.svelte`'s own copy, `api-spec.ts` docs page).

## 2. Shared layout + tab strip

- [x] 2.1 `web/src/routes/my/notifications/+layout.svelte`: render `TabStrip` (`$lib/ui`) with three tabs (History → `/my/notifications`, Search alerts → `/my/notifications/searches`, Settings → `/my/notifications/settings`), `active` derived from `page.url.pathname` (prefix match, default `history`), `onSelect` calling `goto(resolve(...))`; wraps `<slot />` below the strip.
- [x] 2.2 Unit test for the active-tab derivation logic (extract it as a small pure function if that keeps it testable in isolation) covering: exact match on each of the three routes, and the `/my/notifications/:id/jobs` case falling back to `history`.

## 3. History becomes the landing page

- [x] 3.1 Move the content of `web/src/routes/my/notifications/history/+page.svelte` into `web/src/routes/my/notifications/+page.svelte`, replacing today's settings content there.
- [x] 3.2 Delete the old `web/src/routes/my/notifications/history/+page.svelte`; add `web/src/routes/my/notifications/history/+page.ts` with a 308 redirect to `/my/notifications` (mirror `web/src/routes/my/+page.ts`'s pattern).

## 4. Settings moves to its own route

- [x] 4.1 Move today's `web/src/routes/my/notifications/+page.svelte` content (renders `ReminderSettings`) to `web/src/routes/my/notifications/settings/+page.svelte`; drop the now-redundant "looking for what's been sent" link paragraph (the tab strip replaces it).

## 5. Search alerts moves under the section

- [x] 5.1 Move `web/src/routes/my/searches/+page.svelte` content (`<SavedSearchesView />`) to `web/src/routes/my/notifications/searches/+page.svelte`.
- [x] 5.2 Replace `web/src/routes/my/searches/+page.svelte` with a `+page.ts` 308 redirect to `/my/notifications/searches`; remove the old `+page.svelte` in that directory.
- [x] 5.3 Update `ReminderSettings.svelte`'s Telegram hint link (currently pointing at `/my/searches`, labeled "search notifications" page) to the new `/my/notifications/searches` path.

## 6. Nav

- [x] 6.1 `web/src/lib/accountNav.ts`: remove the `{ href: '/my/searches', label: 'Search notifications' }` entry; keep the single `/my/notifications` "Notifications" entry (its comment can drop the now-inaccurate "Distinct from the saved-search alert list below" note).
- [x] 6.2 `web/src/lib/accountNavIcons.ts`: remove the `/my/searches` icon mapping.
- [x] 6.3 Update/remove any test asserting the old nav list (e.g. `accountNav.test.ts` if present) for the removed entry.

## 7. Verification

- [x] 7.1 `pnpm run check` (svelte-kit sync + svelte-check) and `pnpm test` clean on changed/new files.
- [x] 7.2 `pnpm lint` (eslint) clean on changed/new files.
- [x] 7.3 Manual browser check: visit `/my/notifications` (history, tab active), click through to Search alerts and Settings tabs (URL changes, correct tab highlighted); visit `/my/searches` and `/my/notifications/history` directly and confirm both redirect; open a history card's digest detail page and confirm the History tab still shows active; confirm the sidebar shows one "Notifications" entry, none for "Search notifications".
