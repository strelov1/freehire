## Context

`/jobs` is a SvelteKit route whose filter state is URL-first: `JobsView.svelte` builds a `FilterStore` from `page.url.searchParams`, and a `$effect` reloads the list and facet counts (no navigation) whenever the debounced `filters.applied` snapshot changes. Filters serialize to a query string via `facetModel.ts` (`filtersToParams`/`filtersFromParams`) and persist to `localStorage` under `hire.jobFilters` (`filterStorage.ts`); on a soft `/jobs` navigation, `JobsView`'s `afterNavigate` restores them. Facet vocabularies and labels are already exported from `facets.ts`/`labels.ts` and the generated contracts (`CATEGORY_OPTIONS`, `roleLabel`/`ROLE_LABELS`, `WORK_MODE_VALUES`, `SENIORITY_VALUES`, region options). `FilterStore.apply(query)` already seeds the whole store from a query string (used by saved searches).

The onboarding wizard therefore has a clean insertion surface: it can produce a filter query and hand it to the existing store, and the feed reconfigures with zero new plumbing. This slice is deliberately frontend-only and anonymous — no backend, DB, or auth changes.

## Goals / Non-Goals

**Goals:**
- Reconfigure the existing feed from a captured preference set in one confirm, reusing the existing filter store, persistence, and facet counts.
- Keep the wizard a thin front-door: its output is a standard filter query, fully editable/clearable through the existing FilterModal.
- Surface onboarding non-intrusively (banner + persistent entry point) and make it skippable.
- Handle the narrow-feed case honestly (count + relax action), since this slice has no semantic fallback.

**Non-Goals:**
- CV-upload auto-fill, semantic "similar-by-meaning" expansion, Telegram saved-search alert, account/DB persistence, cross-device sync. Each is a later change; their seams are already identified (`/me/resume/extract`, `semantic_ratio`, `/me/searches` + `/me/subscriptions`, `user_profiles`).
- Any change to filter, search, or persistence *requirements*. The wizard consumes them unchanged.

## Decisions

**D1 — Output is a filter query, applied via `FilterStore.apply`, not a new preference model.**
The wizard collects selections in its own staged local state (mirroring how `StagedFilters` isolates edits) and on confirm maps them to a query string through the existing `filtersToParams`, then calls `FilterStore.apply(query)`. *Alternative considered:* a dedicated onboarding preference object threaded into `JobsView`. Rejected — it would duplicate the filter representation and break "editable through the standard filter UI." Reusing the query keeps one source of truth and makes the persistence and FilterModal integration free.

**D2 — Persistence reuses `hire.jobFilters`; onboarding lifecycle is a separate small flag.**
Applying the query naturally flows into the existing `saveJobFilters` path, so the configured feed survives a return visit with no new persistence code. A separate `hire.onboarding` key (`{ state: 'unseen' | 'seen' | 'done' }`) governs only banner visibility, keeping feed state and UI-nudge state decoupled. *Alternative:* overload `hire.jobFilters` presence as the "done" signal. Rejected — a user who clears filters would wrongly re-trigger the banner.

**D3 — A single one-time banner as the only trigger.**
Per product decision, the entry is one unobtrusive banner above the feed (not an auto-opening modal). The banner renders only when: standalone feed **and** `hire.onboarding` ≠ `done`/`seen`-dismissed **and** no active filters in the store. It is the sole entry point — once dismissed or completed it retires; there is no persistent re-open control (deliberately: keep the surface calm, the wizard is a first-run nudge, not a recurring tool). This isolates "should we nudge?" logic in one place.

**D4 — Relax-filter order is fixed by specificity (stack → region → seniority).**
With no semantic fallback in this slice, a narrow selection can empty the feed. The relax action removes the single narrowest applied preference and re-applies, surfacing the change in both feed and filter state. Fixed order beats "remove last-added" (unpredictable) and beats auto-broadening (violates "never silently broaden"). Role is intentionally never auto-dropped — it's the user's primary intent.

**D5 — Component boundaries.**
- `OnboardingWizard.svelte` — owns staged selection + steps; emits `onComplete(query)` / `onSkip()`. Ignorant of URL/localStorage. Pickers built from `facets.ts` vocabularies.
- `OnboardingBanner.svelte` — presentational; knows only shown/hidden and emits open/dismiss.
- `onboarding.ts` — the only impure unit: `selectionsToQuery(selection)` (over `filtersToParams`), the narrowest-facet resolver for relax, and `hire.onboarding` state read/write.
- `JobsView.svelte` — hosts the banner, wires `onComplete` → `filters.apply(query)`, and decides banner visibility. This is the only edit to existing code.

## Risks / Trade-offs

- **[Wizard vocabularies drift from search params]** → Build every picker from the exported `facets.ts`/contracts vocabularies and map exclusively through `filtersToParams`; a unit test asserts `filtersFromParams(selectionsToQuery(s))` round-trips the selection. No hand-written param strings.
- **[Stack values / rendering]** → The stack picker reuses the filter panel's `SearchSelect` fed by the live skills facet distribution, so stack values are drawn from the controlled skills vocabulary (only skills that exist in the catalogue) rather than arbitrary free text — and render as escaped text via Svelte's default `{value}` binding, never raw HTML.
- **[Banner nags users who already filtered or already onboarded]** → Visibility guard keys on both the `hire.onboarding` flag and "no active filters"; dismissing sets `seen` so it will not reappear.
- **[Narrow feed feels dead]** → Honest count + relax action in the existing empty-state; the richer "similar-by-meaning" experience is explicitly deferred, not faked.
- **[localStorage unavailable / private mode]** → Treat storage read/write as best-effort; on failure the wizard still works for the session (feed reconfigures in-memory) and the banner simply falls back to its default (may reappear). No hard failure.

## Migration Plan

Pure additive frontend change — no schema, no API, no data migration. Ships with the web build. Rollback is reverting the `JobsView.svelte` edit and removing the new files; no persisted server state is created. The new `hire.onboarding` localStorage key is inert if the feature is removed.

## Open Questions

- Banner dismissal scope: does "×" suppress for the session or permanently (`seen` persisted)? Leaning permanent-until-cleared for calm UX; final call during implementation, low-risk to flip.
- Exact placement of the persistent "configure feed" control (header vs. filter toolbar) — a UI detail to settle against the current `/jobs` layout during implementation.
