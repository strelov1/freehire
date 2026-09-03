## Context

`web/src/routes/+layout.svelte` already wraps every route and hosts several self-gating overlays that render themselves conditionally from global state: `CvRefreshDialog`, `ConfirmTailorDialog`, `CookieConsent`, `SupportToast`. None of them redirect — they render an absolutely-positioned overlay and read `isAuthenticated()` (`web/src/lib/auth.svelte.ts`) plus their own singleton store.

CV presence is already a singleton `UserResource`: `resumeStore` (`web/src/lib/resume.svelte.ts`) wraps `GET /api/v1/me/resume` and exposes `.meta?.present` plus a lazy `ensureLoaded()`. No new backend signal is needed for the gate condition.

The search profile (`internal/identity/userprofile`, `PUT/GET /api/v1/me/profile`) already has `specializations` and `skills` (validated against `vocab.CategoryValues`) and `location_preferences`. It has no seniority field. The seniority vocabulary (`vocab.SeniorityValues`, 8 values) already exists and is already used, as a multi-select, by the anonymous `/jobs` `OnboardingWizard.svelte` (`sel.seniorities: string[]`) — but that wizard writes to a local filter/localStorage, not the server profile, and has no location step. See proposal.md for the full motivation.

## Goals / Non-Goals

**Goals:**
- Reuse every existing piece that already does what a step needs (CV extraction call, category/skills/location pickers, the resume-presence signal) rather than inventing parallel versions.
- Keep the gate's re-trigger condition equal to "no CV" — no new completion flag, so the behavior is trivially consistent across sessions and devices.

**Non-Goals:**
- No change to the anonymous `/jobs` `OnboardingWizard.svelte` or its localStorage-backed selection — it is a distinct capability (unauthenticated feed preferences) and stays as-is.
- No SSR-level route guard. The gate is a client-side redirect (a root-layout `$effect`, not a `+layout.server.ts`/`load` check); it does not block server rendering of the page it redirects away from.
- No new "role" concept distinct from `specializations` — resolved during brainstorming: what the user calls "role" in this app's own vocabulary already is the specialization/category picker (see the anonymous wizard's "What do you do?" step, which labels the same `CATEGORY_OPTIONS` picker as focus/role).

## Decisions

**Gate mechanism: a dedicated `/onboarding` route with a redirect, not an overlay.**
Superseded during implementation: the overlay-in-`+layout.svelte` approach (matching `CvRefreshDialog` et al.) was built first, per the original brainstorming answer, then replaced with a real route at the user's explicit request for a proper URL. The root layout (`web/src/routes/+layout.svelte`) now runs two effects: one redirects an authenticated, CV-less, not-yet-dismissed-this-visit user to `/onboarding` from anywhere else (fully reactive — fires again on the next navigation, or the next reactive change, as long as the condition holds); the other redirects AWAY from `/onboarding` (CV now present, or signed out), but only at the moment of *arriving* there — it reads `resumeStore`/`isAuthenticated`/`onboardingGate` inside `untrack()` so it is not re-triggered by those values changing while already on the page. That split exists because of a real bug caught live (see Risks): without it, the CV step's own upload flips `resumeStore.present` mid-visit (on purpose, so a later visit skips the gate), and a fully-reactive away-redirect would bounce the user off the page before they ever reached the confirm/location steps.

`onboardingGate` (`web/src/lib/onboardingGate.svelte.ts`) is a small module-level `$state` singleton (mirrors `auth-dialog.svelte.ts`) recording "has this visit already left `/onboarding`". Without it, finishing or skipping the wizard (which navigates to `/`) would have the layout's to-`/onboarding` effect immediately re-fire (CV may still be absent) and bounce the user right back — the same anti-loop `dismissed` served in the overlay version, just now scoped to a visit rather than a component instance.

**New component, not an extension of `OnboardingWizard.svelte`.**
The two wizards diverge on persistence target (server profile vs. local filter), audience (authenticated-only vs. anonymous-allowed), route (dedicated full-screen page vs. centered modal on `/jobs`), and step count (3 vs. 2, the extra step being location). Branching one component on all of those would leave more conditional plumbing than the ~150 lines of picker UI it would save. `web/src/routes/onboarding/+page.svelte` reuses the *pattern* (staged local selection object, pill-group snippets, CV-auto-fill-then-stay-on-step-for-review) but is its own file — with no gating logic of its own; that all lives in the layout now.

**Role picker: a search input (`RemoteSearchSelect`), not a pill grid.**
Changed during implementation, on user feedback after seeing the confirm step live: `CATEGORY_OPTIONS` is ~35 values, and a full pill grid for it was unwieldy next to Skills' compact search box. `CATEGORY_OPTIONS` is a short, static, already-loaded list, so the search function is synchronous filtering wrapped in a resolved promise — same shape as the Skills field's live-dictionary search, so both fields share one component and one interaction pattern. Level (8 seniority values) stayed as pills; a grid that size doesn't have the same problem.

**Per-field "Clear" control, matching `FacetSection`'s pattern.**
Also added on user feedback: each of Role/Level/Skills gets a small X next to its label, shown only once something is selected, that clears the whole field at once — the same header affordance `FacetSection.svelte` uses for every facet in the standard filter UI. This is separate from removing one value at a time (a chip's own X, or clicking a pressed pill again).

**Backend field: `seniorities text[]` on `user_profiles`, mirroring `specializations`.**
Same shape, same validation style (dictionary membership, dedup preserving order) as the existing `specializations` column — the smallest change consistent with how the table already models a multi-valued controlled-vocabulary field. No cap is applied (unlike `specializations`' 5-entry cap): the vocabulary itself only has 8 values, so a cap would be redundant.

**Commit once, at wizard close, through the existing `PUT /api/v1/me/profile`.**
No new endpoint and no per-step persistence. This matches how the anonymous wizard already stages selections locally (`sel` state) and commits only in `complete()`. The CV file itself is the one exception — it persists immediately on upload through the existing extraction call, because that call's contract (`api.extractResumeProfile`) already performs the upload as a side effect of extraction; there is no "stage the file, upload later" mode to reuse instead. The commit guard is `specializations.length === 0 || skills.length === 0` (skip unless BOTH are non-empty) — the save endpoint's existing validation rejects either being empty, so an `&&` guard would let a role-only or skills-only save through to a 400 the user never asked for and can't see (the gate has already closed by then).

**Pre-fill from the existing profile, not just from CV extraction.**
Since the gate's re-trigger condition is "no CV" rather than "onboarding incomplete," a user can see the overlay again after a visit where they filled in role/skills/level but skipped the CV step. Re-fetching their saved profile (`profileStore`) to pre-populate the confirm/location steps avoids silently discarding that input on the next visit.

## Risks / Trade-offs

- **[Risk]** A signed-in user with no CV is redirected to `/onboarding` on every visit, including one started from a page unrelated to profile/CV (e.g. a single job posting). This was explicitly requested and confirmed twice during brainstorming (re-trigger "on every visit," applying to "old and new" users alike) — flagged here for the record, not as an open question.
- **[Risk — found live, fixed]** Both the overlay's original `visible` condition and the route's original single redirect effect were fully reactive functions of `resumeStore.present`. Since the CV step's own upload flips that flag as a side effect (`resumeStore.noteUpload()`), the gate closed / the user was bounced off the page the instant a CV finished uploading — before ever reaching the confirm or location steps. Fixed twice, once per architecture: the overlay version latched an `opened` flag once true and never re-derived it; the route version reads the away-redirect's condition inside `untrack()` so it only re-evaluates on an actual navigation, not on every `resumeStore.present` change.
- **[Risk]** Adding `seniorities` to the profile write path changes an existing endpoint's request/response shape. → Mitigation: the field is optional and additive (defaults to empty array, same treatment as `excluded_skills`), so existing clients that omit it are unaffected.
- **[Trade-off]** The redirect is client-side only (no SSR-level route guard / `+layout.server.ts` check); a user with JS disabled, or mid-hydration, briefly sees the destination page before the redirect fires. Consistent with how the rest of this app's client-only gating (the self-gating dialogs this design started from) already behaves — not a new limitation.

## Post-implementation scope expansion (not reflected in specs/tasks above)

During live review the user asked for three more things, implemented but not written back into the formal spec/task artifacts (documented here instead, given the size of the change already made):

1. **`/onboarding` absorbs registration entirely.** `AuthDialog.svelte` no longer has a `register` mode — its "No account? Create one" link, and `JobView.svelte`'s signed-out apply/track gate's "Sign up" button, both navigate to `/onboarding?returnTo=<current path>` instead. `/onboarding` gained a conditional first step, `auth` (register by default, with an inline "Sign in instead" toggle — no separate dialog), shown only while signed out; the step list (`stepKinds`) is derived from `isAuthenticated()`, so a successful register/login mid-page drops `auth` out of the list and the SAME page carries straight on into the CV step, no navigation. `web/src/lib/auth-dialog.svelte.ts`'s `Mode` type dropped `'register'` accordingly.
2. **The anonymous `/jobs` onboarding wizard is retired.** `OnboardingWizard.svelte` is deleted; `JobsView.svelte`'s banner ("Make this your feed" / "Set up") now navigates to `/onboarding?returnTo=<current path>` (marking the local nudge lifecycle `seen`, same as dismissing it) instead of opening the local wizard that wrote to the anonymous filter query. This is a deliberate behavior change: configuring the feed without an account is no longer possible. `onboarding.ts`'s `narrowestFacet`/lifecycle helpers (still used by the "relax narrowest facet" empty-state action and the banner) are untouched; `selectionsToQuery`/`emptySelection`/`OnboardingSelection` and `markDone` are left in place, fully tested, but have no live caller anymore — a deliberate deferred cleanup given the size of this change (their tests double as `narrowestFacet`'s fixtures; splitting that out is follow-up work, not done here).
3. **`returnTo` threading.** Every entry point to `/onboarding` carries `?returnTo=<safeRedirect-validated path>` (helper: `onboardingUrl()` in `onboardingGate.svelte.ts`), and both ways of leaving the page (the auth step's "Skip for now"/close, and `finish()` from the cv/confirm/location steps) navigate there instead of a hardcoded `/`. The root layout's auto-redirect (no-CV gate) and its away-redirect (CV now present) were updated to match: the away-redirect also honors a `returnTo` on the URL, and — since an anonymous visitor legitimately belongs on `/onboarding` now (the auth step) — it only fires when signed in AND a CV already exists, not merely "signed out."
4. **Two UI changes on the confirm step, from live feedback**: Role became a `RemoteSearchSelect` (search-as-you-type over the static `CATEGORY_OPTIONS` list) instead of a ~35-pill grid, matching Skills' existing pattern; each of Role/Level/Skills gained a per-field "Clear" X (shown once populated) matching `FacetSection.svelte`'s section-header pattern elsewhere in the app.
5. **Auth-step visual design**: a two-column layout (dark brand panel + real, non-fabricated product copy on the left; the credential form on the right), at the user's explicit request, modeled on a reference screenshot of a third-party product's login page but mirrored (form right, not left) and restyled with this app's own tokens/copy — no borrowed branding, logo, or fabricated testimonial.

None of this touched the backend (`seniorities` field, migration, handler) described above — that part shipped exactly as designed.

## Migration Plan

1. Migration `0123_user_profile_seniorities.sql` adds the column (nullable/default-empty array, no backfill needed — every existing row simply has no seniorities yet, which is valid).
2. Backend (sqlc regen, `userprofile` package, handler) deploys first; the new field is additive so the existing frontend keeps working unmodified against it.
3. Frontend (new field in types/api/store, new overlay component, layout wiring) deploys after the backend field is live, so the wizard's save call always has somewhere to write `seniorities`.
4. No rollback complexity beyond the standard migration/deploy reversal — nothing destructive, no backfill, no data migrated from another shape.
