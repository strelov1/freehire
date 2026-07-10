## Why

A signed-in user who has filled in their profile (specializations + skills) still has
to re-pick those same values by hand every time they open the job filters. The profile
already encodes "the work I'm looking for" — the filter modal should be able to apply it
in one action instead of making the user reconstruct it facet by facet.

## What Changes

- Add an **Apply my profile** action to the header of the jobs filter modal that, in one
  click, resets the staged filters and seeds them from the signed-in user's profile:
  `specializations → category` facet and `skills → skills` facet.
- The action stages only — nothing reaches the job list until the existing **Show
  results** commit, so the user previews the profile-derived filters (and can adjust
  them) before applying, consistent with the modal's deferred-apply contract.
- The action is shown only on the full jobs filter modal for a signed-in user who has a
  saved profile. When the signed-in user has **no** profile, the header instead offers a
  link to create one (`/my/profile`); for signed-out users nothing is shown.
- Frontend-only. Reuses the existing `profileStore`, the `StagedFilters` store, and the
  established `specializations → category` / `skills → skills` mapping. No backend, API,
  or schema changes.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `filter-modal`: adds a requirement for a header action that resets and seeds the staged
  filters from the signed-in user's profile, with a create-profile fallback.

## Impact

- `web/src/lib/components/filters/FilterModal.svelte` — owns the profile-apply logic
  (job-specific).
- `web/src/lib/components/filters/FilterModalShell.svelte` — gains one optional header
  snippet slot so the domain wrapper can inject the action; stays domain-agnostic
  (unchanged for `CompanyFilterModal`).
- Reuses `web/src/lib/profile.svelte.ts` (`profileStore`) and
  `web/src/lib/stagedFilters.svelte.ts` (`StagedFilters.clear` + facet seeding).
- No backend, database, or generated-contract changes.
