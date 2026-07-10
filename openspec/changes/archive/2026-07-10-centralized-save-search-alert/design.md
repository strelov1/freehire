## Context

The save + Telegram-alert flow already works but lives inline in `web/src/lib/components/SavedSearches.svelte` (the FilterModal "My filters" tab): a save bar (`save()`/`update()`), the notify controls (`toggleNotify`/`connectTelegram`/`recheckLink`), and dedupe against the current query via `canonicalQuery`. The backend is complete and unchanged: `savedSearches.svelte.ts` (`create`/`ensureLoaded`), `notifications.svelte.ts` (`link`/`refreshTelegram`/`subscribe`/`telegram` status/`forSavedSearch`), the `/me/searches` · `/me/subscriptions` · `/me/telegram` endpoints, and the notify worker. Telegram account linking (deep link → webhook → `telegram_links`) is fully implemented and user-facing.

The just-shipped onboarding wizard emits a filter query (`selectionsToQuery`) in the exact canonical format the `saved_searches.query` column stores. This change makes the save+alert a first-class, query-driven unit reused from three surfaces instead of one.

## Goals / Non-Goals

**Goals:**
- One implementation of "save this search + get it in Telegram," reachable from the sidebar, the post-onboarding banner, and the modal tab.
- The alert is the primary action and implies the save (auto-name, dedupe) — one tap for the common case.
- Robust for the anonymous-onboarding path: auth gate + resume across an OAuth full-page redirect.

**Non-Goals:**
- No backend/DB/worker/endpoint changes; no email channel, digest-frequency settings, or board sharing (stays on `/my/searches`).
- Not changing the requirements of `saved-searches`/`filter-subscriptions`; the modal tab keeps its list management (apply/rename/delete/"Update <name>").

## Decisions

**D1 — The unit is a query-driven module + compact component, not a store-coupled widget.**
`saveSearchAlert.ts` owns the orchestration over the existing stores and takes a **query string**, so it is agnostic to staged (modal) vs. live (sidebar/onboarding) filters. `SaveSearchAlert.svelte` renders the states from a `query` prop. *Alternative:* generalize `SavedSearches.svelte` in place. Rejected — it is bound to `StagedFilters` and mixes list management with the save+alert concern; a query-string boundary is what lets all three surfaces share it.

**D2 — Save first, alert second (two distinct steps).**
Saving is the primary, standalone action — it works without any subscription — and the Telegram alert is offered only once the search is saved. Save auto-names a new set (`alertName(query)` from the existing facet labels) and dedupes by `canonicalQuery`; the alert step (link + subscribe) then targets that saved set. *Alternative (initially built, then rejected on review):* one "get new jobs in Telegram" tap that implicitly saves. Rejected — merging save and subscribe was confusing (especially in the modal's "My filters" tab), and it hid that saving is useful on its own. Splitting them matches the mental model and the original saved-searches behavior; the modal keeps explicit rename and "Update <name>".

**D3 — OAuth resume via a `hire.pendingAlert` handoff.**
Onboarding runs anonymously and OAuth sign-in is a full-page redirect, so in-memory intent is lost. Before opening the auth dialog the flow records the query in `hire.pendingAlert`; `/jobs` mount, when the user is authenticated and a pending record exists, replays `enableAlert(query)` and clears the record. The feed itself is already restored from `hire.jobFilters`, so the replay reconstructs the full intent. Password sign-in (in-dialog, no navigation) continues immediately without the handoff. *Alternative:* only offer the alert to already-signed-in users. Rejected — most onboarding users are anonymous, so reach would be near-zero.

**D4 — Component boundaries.**
- `saveSearchAlert.ts` — pure-ish orchestration: `enableAlert(query)` (auth → resolve-or-create saved search → link → subscribe, idempotent), `alertName(query)`, `alertStateFor(query)` (signed-out / not-saved / saved-not-linked / connecting / subscribed), and `pendingAlert` set/consume/clear over `localStorage`. No Svelte markup; unit-testable with faked stores.
- `SaveSearchAlert.svelte` — compact presentational states over `saveSearchAlert.ts`, `query` + `variant` props; no knowledge of which surface hosts it.
- `SavedSearches.svelte` — keeps list/apply/rename/delete/"Update"; renders `SaveSearchAlert` for save + notify.
- `FilterSummary`/`FilterSummaryShell` — a "Save filter" affordance under "All filters" that reveals `SaveSearchAlert` for the live query.
- `JobsView.svelte` — hosts the post-onboarding banner and runs the `pendingAlert` resume on mount.

## Risks / Trade-offs

- **[Refactoring the working modal tab regresses save/notify]** → Extract behavior-preservingly: `SaveSearchAlert` reproduces the existing states; the modal keeps its list logic. Covered by re-verifying the modal tab end-to-end and unit tests on `saveSearchAlert.ts`.
- **[Auto-name collides with `UNIQUE(user_id, name)`]** → Resolve-by-query first (reuse the existing set); only create when no query match, and make `alertName` include distinguishing facets. A residual 409 on create is caught and retried against the just-created/looked-up set, treated as success.
- **[`pendingAlert` replays stale/unwanted]** → The record is consumed (read-and-clear) exactly once on mount and only when authenticated; a signed-out load or absent record is a no-op. Keyed to the query, not a boolean.
- **[Telegram link recheck races / user never taps Start]** → `connecting` state with a manual "I've connected" re-check and no auto-poll storm; the page stays usable and the subscription simply isn't created until linked (matching current behavior).
- **[Telegram disabled server-side]** → `telegram.enabled === false` hides the alert affordance everywhere; saving a search still works where that surface offers it.

## Migration Plan

Pure additive/refactor frontend change — no schema, API, or data migration. Ships with the web build; rollback is reverting the touched files and removing the new ones. The new `hire.pendingAlert` localStorage key is inert if the feature is removed.

## Open Questions

- Sidebar "Save filter" presentation: inline expander vs. small popover — a UI detail settled against the current sidebar layout during implementation; low-risk either way.
- Auto-name format (which facets to include, truncation) — refined during implementation against real queries; the dedupe-by-query guard makes the exact string non-load-bearing.
