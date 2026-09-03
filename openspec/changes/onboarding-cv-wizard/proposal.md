## Why

New accounts land in the app with no CV and no profile, and nothing today prompts them to fix that. The base résumé (`ProfileForm.svelte` on `/my/profile`) and the search profile (`specializations`/`skills`/`location_preferences`) only get filled if a user finds that page on their own. A forced, skippable full-screen step right after sign-in — while the account has no CV — turns that into an active prompt instead of a page nobody visits.

## What Changes

- Add a full-screen onboarding page at `/onboarding`, with a client-side redirect in the root layout sending any authenticated user who has no CV yet (`GET /api/v1/me/resume` → `present: false`) there from anywhere else. It reappears on every visit until a CV is uploaded — there is no separate "onboarding completed" flag.
- The page is a 3-step wizard, each step skippable, staged locally and committed once at the end via the existing `PUT /api/v1/me/profile`:
  1. **CV** — upload, reusing the existing extraction call (`api.extractResumeProfile`).
  2. **Confirm** — three multi-select pickers: Role (`specializations`), Skills (`skills`), Level (`seniorities`, new).
  3. **Location** — reuses the existing `location_preferences` picker.
- Fields pre-fill from CV extraction (if step 1 ran) or from the user's existing saved profile, so skipping one visit doesn't discard what was entered on a previous one.
- **BREAKING**: none — additive UI and an additive, optional profile field.

## Capabilities

### New Capabilities
- `post-registration-onboarding`: the gated full-screen wizard page — trigger condition, step behavior, skip semantics, persistence-across-visits, and how it hands off to the existing profile save.

### Modified Capabilities
- `search-profiles`: adds an optional `seniorities` field (multi-select, drawn from the seniority vocabulary) to the profile read/write contract.

## Impact

- **Backend**: new migration `0123_user_profile_seniorities.sql` (`user_profiles.seniorities text[]`); `internal/platform/db/queries/user_profiles.sql` + sqlc regen; `internal/identity/userprofile/userprofile.go` (`Profile.Seniorities`, validation against `vocab.SeniorityValues`, `Service.Save` signature); `internal/api/handler/me_profile.go` (request/response field).
- **Frontend**: new route `web/src/routes/onboarding/+page.svelte`; a redirect effect pair added to `web/src/routes/+layout.svelte` (alongside the existing self-gating dialogs); a new shared flag `web/src/lib/onboardingGate.svelte.ts`; `web/src/lib/types.ts` (`UserProfile.seniorities`), `web/src/lib/api.ts`, `web/src/lib/profile.svelte.ts`.
- **No changes** to the existing anonymous `/jobs` `OnboardingWizard.svelte` — only UI patterns (pill groups, CV auto-fill) are reused, not the component itself.
