// Onboarding: the pure glue between the /jobs onboarding wizard and the existing
// job filters, plus the local "have we nudged this visitor yet?" lifecycle. The
// wizard captures coarse feed preferences; here they map onto the SAME facet
// filter query the standard filter UI produces, so the wizard is a front-door to
// existing filters, not a parallel system. No Svelte here (mirrors facetModel.ts /
// filterStorage.ts), so it's unit-testable in plain Node and every storage access
// is best-effort — private mode / SSR must never break the feed.

import { emptyFilters, filtersToParams, type JobFilters } from './facetModel';

// The wizard's captured preferences. Every field is optional: the wizard is
// skippable per field, and an empty field contributes no filter constraint.
// Each maps to one existing job facet param (see selectionsToQuery).
export interface OnboardingSelection {
  /** Specializations → the `category` facet (Backend, Frontend, ML/AI, …). Multi-select:
   *  a person can be several (a CV upload can pre-fill more than one). */
  specializations: string[];
  /** Seniorities → the `seniority` facet. Multi-select: a search can span a grade range
   *  (Middle + Senior), though a CV pre-fills the single current grade. */
  seniorities: string[];
  /** → the `work_mode` facet. */
  workMode?: string;
  /** → the `regions` facet (macro region code). */
  region?: string;
  /** Free-text stack tokens → the `skills` facet. */
  stack: string[];
}

export function emptySelection(): OnboardingSelection {
  return { specializations: [], seniorities: [], stack: [] };
}

/** Map the wizard's selection onto the existing filter query string, via the same
 *  `filtersToParams` the standard filter UI uses — so an empty selection yields an
 *  empty query and a full one yields exactly what the equivalent facet clicks would.
 *  No hand-written param strings: the facet-param contract stays in facetModel. */
export function selectionsToQuery(sel: OnboardingSelection): string {
  const f = emptyFilters();
  if (sel.specializations.length) f.facets.category!.include = [...sel.specializations];
  if (sel.seniorities.length) f.facets.seniority!.include = [...sel.seniorities];
  if (sel.workMode) f.facets.work_mode!.include = [sel.workMode];
  if (sel.region) f.facets.regions!.include = [sel.region];
  const stack = sel.stack.map((s) => s.trim()).filter(Boolean);
  if (stack.length) f.facets.skills!.include = stack;
  return filtersToParams(f).toString();
}

// The facets the narrow-feed "relax" action peels off, narrowest (most specific)
// first. The role/specialization is deliberately absent — it's the visitor's
// primary intent and is never auto-dropped. Operates on the live filter state so
// the affordance works for any narrow feed, not only a wizard-built one.
export const RELAX_FACET_ORDER = ['skills', 'regions', 'seniority'] as const;

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

/** unseen = never nudged (banner may show); seen = dismissed without completing
 *  (banner suppressed); done = completed the wizard (banner suppressed for good). */
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

/** Record that the visitor dismissed the nudge without completing it. Never
 *  downgrades a completed onboarding (done outranks seen). */
export function markSeen(): void {
  if (loadOnboardingState() !== 'done') save('seen');
}

/** Record that the visitor completed the wizard — the banner won't show again. */
export function markDone(): void {
  save('done');
}
