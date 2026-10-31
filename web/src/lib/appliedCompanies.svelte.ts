// Tracks which employers the signed-in user has applied to, so a catalogue card and the
// job page can say "you already applied to this company" for ANY posting of theirs, not
// only the one applied through. The set of company slugs is read once from
// GET /api/v1/me/tracking/applied-companies (the browse view and the job page trigger the
// load) and compared against each posting's `company_slug` client-side, so the public job
// reads stay unauthenticated — the same arrangement as viewedJobs and savedJobs.
//
// The server decides what counts (an application actually made, keyed by the posting's
// current slug and resolved through the alias registry); this store only remembers the
// answer. Marking applied on the job page adds the employer locally, so other postings of
// theirs pick the badge up without a reload.
//
// SSR-safe and auth-agnostic (see UserResource): the load is a browser-only no-op and the
// set stays empty for signed-out users. A failed load leaves the set empty — no badge,
// the correct degraded state: a missing "you applied here" is far cheaper than a wrong one.

import { api } from '#lib/api.js';
import { SlugSet } from '#lib/userResource.svelte.js';

class AppliedCompanies extends SlugSet {
  protected load(): Promise<string[]> {
    return api.listAppliedCompanySlugs();
  }
}

const appliedCompanies = new AppliedCompanies();

/** Whether the signed-in user has applied to any posting of this employer. A posting
 *  with no company slug never matches: an empty slug names no employer. */
export function hasAppliedToCompany(companySlug: string | undefined): boolean {
  return !!companySlug && appliedCompanies.has(companySlug);
}

/** Record a fresh application's employer locally, after the write succeeded. */
export function markAppliedToCompany(companySlug: string | undefined) {
  if (companySlug) appliedCompanies.mark(companySlug);
}

export function ensureAppliedCompaniesLoaded(): Promise<void> {
  return appliedCompanies.ensureLoaded();
}
