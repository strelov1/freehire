## Why

Saving a search and turning on its Telegram alert is the product's retention loop, but today it lives in exactly one place — the FilterModal's "My filters" tab (`SavedSearches.svelte`), where a user has to open the modal, switch tabs, type a name, then find the notify toggle. The just-shipped onboarding wizard configures a feed at peak intent but offers no way to keep it. We want one centralized "save this search + get it in Telegram" affordance, surfaced right where intent is highest — the filters sidebar and the post-onboarding moment — reusing the existing save/subscription/Telegram-link plumbing rather than duplicating it.

## What Changes

- Extract the save + Telegram-alert flow into **one shared module** (`saveSearchAlert.ts`, orchestration) and **one compact component** (`SaveSearchAlert.svelte`, UI), driven by a query string.
- Reframe the primary action as **"get new jobs in Telegram," where the alert implies the save**: it saves the search if new (auto-named, deduped by canonical query), links Telegram if needed, then subscribes — one tap. Explicit naming/rename stays as a secondary affordance for list management.
- Surface it from **three places, one implementation**:
  1. A restored **"Save filter"** button in the `/jobs` sidebar (under "All filters") for the live filters.
  2. An **ephemeral post-onboarding banner** for the wizard's query.
  3. The existing **FilterModal "My filters" tab**, refactored to render the shared component for its save + notify controls (keeping its list: apply / rename / delete / "Update <name>").
- Handle **auth gating** (`openAuthDialog`) and **OAuth-redirect resume** (a `hire.pendingAlert` localStorage handoff replayed on `/jobs` mount), the **Telegram link → recheck → subscribe** step, `telegram.enabled` gating, and idempotent 409s (already-saved / already-subscribed → success).
- **Frontend only.** Reuses `POST /me/searches`, `POST /me/subscriptions`, `/me/telegram/*` unchanged — no backend, DB, or worker changes.

## Capabilities

### New Capabilities
- `save-search-alert`: A centralized, query-driven "save this search and get its new jobs in Telegram" flow — a shared module + compact component surfaced from the filters sidebar, the post-onboarding banner, and the saved-searches modal tab, over the existing saved-search / subscription / Telegram-link backend.

### Modified Capabilities
<!-- None. Reuses saved-searches, filter-subscriptions, and jobs-onboarding behavior without changing their requirements; the onboarding banner and sidebar button are additive surfaces. -->

## Impact

- **Frontend only** (`web/`):
  - New `web/src/lib/saveSearchAlert.ts` (orchestration + `alertName` + `pendingAlert` handoff + `alertStateFor`), `web/src/lib/components/filters/SaveSearchAlert.svelte` (compact UI).
  - Edits: `SavedSearches.svelte` (delegate save + notify to the shared component, keep list/dirty-update), `FilterSummary.svelte`/`FilterSummaryShell.svelte` (the "Save filter" button under "All filters"), `JobsView.svelte` (post-onboarding banner host + `pendingAlert` resume on mount), `OnboardingBanner`/onboarding wiring.
- **Reuses, does not change:** `savedSearches.svelte.ts`, `notifications.svelte.ts`, `auth.svelte.ts`/`auth-dialog.svelte.ts`, `canonicalQuery`/`filtersToParams`, and the `/me/searches` · `/me/subscriptions` · `/me/telegram` APIs.
- New `localStorage` key `hire.pendingAlert`. No backend, DB migration, new endpoints, or new dependencies.
