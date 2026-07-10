## Why

The `/jobs` page greets every visitor with a reverse-chronological wall of the whole catalogue — postings in mixed languages, C-level next to junior, no relevance. The landing page promises *"every tech job in one clean feed"*, but the feed a user actually lands on is neither clean nor theirs. Nothing prompts a first-time visitor to narrow it, so the product's normalization and facets (role/seniority/geo/skills) go unfelt. A short, skippable onboarding turns the wall into "your feed" in ~15 seconds, using filters that already exist.

## What Changes

- Add a 3-step onboarding wizard to `/jobs` (frontend-only): **step 1** role + seniority, **step 2** work-mode + region + stack, **step 3** payoff (feed reconfigures, wizard closes).
- The wizard's selections map to the **existing facet filter query** and are applied through the existing `FilterStore` — the wizard is a friendly front-door to current filters, not a parallel system. Re-editing or clearing via the existing FilterModal keeps working unchanged.
- Trigger: an **unobtrusive banner above the feed** ("Собрать твою ленту →") that expands the wizard, plus a **persistent "Настроить ленту" entry point**. The banner shows only on a fresh `/jobs` (no active filters, onboarding not yet completed).
- Persist the resulting filter set to `localStorage` via the existing `hire.jobFilters` mechanism, plus a small `hire.onboarding` flag (seen/done) that controls banner visibility. No account required.
- Empty/narrow feed: when the chosen filters yield few or zero jobs, show the honest count and an **"ослабить фильтр"** action that drops the narrowest facet (stack → region → seniority).
- **Out of scope for this slice** (each a later change): CV-upload auto-fill, semantic "similar-by-meaning" expansion, Telegram saved-search alert, persisting preferences to the account/DB, cross-device sync.

## Capabilities

### New Capabilities
- `jobs-onboarding`: A skippable, anonymous onboarding wizard on `/jobs` that captures role/seniority/work-mode/region/stack (or is dismissed), reconfigures the existing job feed through the existing facet filters, persists the result to `localStorage`, and is surfaced by a banner plus a persistent entry point. Includes the narrow-feed "relax filter" affordance.

### Modified Capabilities
<!-- None. The wizard reuses filter-persistence, filter-modal, and job-search behavior without changing their requirements. -->

## Impact

- **Frontend only** (`web/`): new `web/src/lib/components/onboarding/OnboardingWizard.svelte`, `OnboardingBanner.svelte`, and a `web/src/lib/onboarding.ts` helper (selection→query mapping + localStorage state); a small edit to `web/src/lib/components/JobsView.svelte` to host the banner and apply the wizard's query.
- **Reuses, does not change:** `web/src/lib/facets.ts` vocabularies (`CATEGORY_OPTIONS`, `roleLabel`, `WORK_MODE_VALUES`, `SENIORITY_VALUES`, region options), `web/src/lib/facetModel.ts` (`filtersToParams`/`filtersFromParams`), `web/src/lib/filters.ts` (`FilterStore.apply`), `web/src/lib/filterStorage.ts` (`hire.jobFilters`).
- **No backend, no DB migration, no new API endpoints, no new dependencies.**
- New `localStorage` key `hire.onboarding`; the existing `hire.jobFilters` key is written through its existing helper.
