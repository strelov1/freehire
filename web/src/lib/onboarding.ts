// Onboarding: what's left of the local "have we nudged this visitor yet?" nudge for
// the /jobs feed, now that the wizard it used to gate (a coarse role/seniority/work-
// mode/region/stack picker writing straight to the filter query) has been retired in
// favor of sending visitors to /onboarding (see onboardingGate.svelte.ts) — that flow
// writes a real server profile, not a local filter query, so there is nothing here for
// it to reuse. `narrowestFacet` and the lifecycle (bannerVisible/loadOnboardingState/
// markSeen) are still live: JobsView reads them directly for the empty-state "relax
// narrowest facet" action and the onboarding banner's own dismiss/re-show rule. No
// Svelte here (mirrors facetModel.ts / filterStorage.ts), so it's unit-testable in
// plain Node and every storage access is best-effort — private mode / SSR must never
// break the feed.
//
// WHAT IS NOT HERE, deliberately: whether the account completed onboarding. That is an
// explicit server-side fact (`users.onboarding_completed_at`, read off the user in the
// root layout), because it has to follow the person rather than the browser — a candidate
// who finished the wizard on their laptop must not be walked through it again on their
// phone, and clearing site data must not re-ask a completed account. What lives here is
// only the feed BANNER's dismissed/not-dismissed nudge, which is a per-browser
// presentation concern and correctly forgotten with the storage.

import type { JobFilters } from './facetModel';

// The facets the narrow-feed "relax" action peels off, narrowest (most specific)
// first. The role/specialization is deliberately absent — it's the visitor's
// primary intent and is never auto-dropped. Operates on the live filter state so
// the affordance works for any narrow feed, not only a wizard-built one.
const RELAX_FACET_ORDER = ['skills', 'regions', 'seniority'] as const;

/** The single narrowest relaxable facet currently constraining the feed, or null
 *  if none is set. The empty-state relax action clears exactly this one. */
export function narrowestFacet(f: JobFilters): string | null {
  for (const param of RELAX_FACET_ORDER) {
    const st = f.facets[param];
    if (st && (st.include.length > 0 || st.exclude.length > 0)) return param;
  }
  return null;
}

// ---- lifecycle: the local nudge state ----

export const ONBOARDING_KEY = 'hire.onboarding';

/** unseen = never nudged (banner may show); seen = dismissed (banner suppressed).
 *  'done' is a legacy third value: the retired wizard used to write it on completion,
 *  and a browser that stored it before this change must keep reading it correctly
 *  (same suppression as 'seen') — nothing writes it going forward. */
export type OnboardingLifecycle = 'unseen' | 'seen' | 'done';

/** The banner shows only to a visitor we've never nudged AND who has no active
 *  filters (so a shared filtered link is never interrupted). Pure for testability. */
export function bannerVisible(state: OnboardingLifecycle, hasActiveFilters: boolean): boolean {
  return state === 'unseen' && !hasActiveFilters;
}

/** The stored lifecycle, defaulting to 'unseen' when absent/unavailable. */
export function loadOnboardingState(): OnboardingLifecycle {
  if (typeof localStorage === 'undefined') return 'unseen';
  try {
    const v = localStorage.getItem(ONBOARDING_KEY);
    return v === 'seen' || v === 'done' ? v : 'unseen';
  } catch {
    return 'unseen';
  }
}

function save(state: OnboardingLifecycle): void {
  if (typeof localStorage === 'undefined') return;
  try {
    if (state === 'unseen') localStorage.removeItem(ONBOARDING_KEY);
    else localStorage.setItem(ONBOARDING_KEY, state);
  } catch {
    // best-effort: private mode / quota / disabled storage
  }
}

/** Record that the visitor dismissed the nudge. Never downgrades a legacy 'done'
 *  value (a completed run of the retired wizard) back to 'seen' — both suppress the
 *  banner identically, so this only matters for not losing information on re-save. */
export function markSeen(): void {
  if (loadOnboardingState() !== 'done') save('seen');
}
