## 1. CompanyFollowButton component

- [x] 1.1 Create `web/src/lib/components/CompanyFollowButton.svelte` with props `{ slug, companyName }` and the derived follow state: `companyQuery = canonicalQuery("company_slug=<slug>")`, `savedSearch = savedSearches.items.find(...)`, `sub = notifications.forSavedSearch(...)`, `subscribed`.
- [x] 1.2 Add the SSR-safe `$effect` that calls `savedSearches.ensureLoaded()` + `notifications.ensureLoaded()` when `isAuthenticated()`, mirroring `SavedSearches.svelte`.
- [x] 1.3 Implement the `toggle` handler: signed-out → `openAuthDialog()`; not-linked Telegram → connect deep-link flow; not-following → reuse-or-create saved search (name = companyName) then `notifications.subscribe`; following → `notifications.unsubscribe` then name-guarded `savedSearches.remove`.
- [x] 1.4 Implement the Telegram connect + "I've connected" re-check affordance (`notifications.link()` / `refreshTelegram()`), plus local `busy`/`error` state, matching the `SavedSearches.svelte` pattern.
- [x] 1.5 Render: hidden when signed-in and `!telegram.enabled`; button label "Subscribe to updates" (Bell) ⇄ "Subscribed"; error text; connect/recheck states.

## 2. Mount on the company page

- [x] 2.1 Mount `CompanyFollowButton` in `web/src/lib/components/CompanyView.svelte` header (right of the title, `ml-auto`), passing `slug` and `company.name`.

## 3. Verify

- [x] 3.1 Run `npx svelte-check` in `web/` and confirm the change introduces no new errors.
- [x] 3.2 Manual/visual pass: signed-out (auth dialog), signed-in follow → "Subscribed", unfollow toggles back; confirm no duplicate saved search on re-follow.
