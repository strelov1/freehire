// Persists the standalone /jobs filter set in browser storage as the serialized
// filter query string, so it survives a navigation back to a bare /jobs. The URL
// stays the source of truth; this is only the "last explicit filter set" mirror,
// restored by JobsView when the user lands on /jobs with no filter params.
//
// Self-contained on purpose: it feature-detects `localStorage` (typeof) rather
// than importing `browser` from `$app/environment`, so the unit test runs in the
// plain-Node vitest env (which has no SvelteKit runtime). Every access is wrapped
// and failures are swallowed — private mode / quota / disabled storage must never
// break filtering, which still works from the URL.

export const JOB_FILTERS_KEY = 'hire.jobFilters';

/** The stored filter query string, or '' when absent/unavailable. */
export function loadJobFilters(): string {
  if (typeof localStorage === 'undefined') return '';
  try {
    return localStorage.getItem(JOB_FILTERS_KEY) ?? '';
  } catch {
    return '';
  }
}

/** Mirror the applied filters to storage. An empty string removes the key, so a
 *  cleared filter set leaves nothing to restore. */
export function saveJobFilters(qs: string): void {
  if (typeof localStorage === 'undefined') return;
  try {
    if (qs) localStorage.setItem(JOB_FILTERS_KEY, qs);
    else localStorage.removeItem(JOB_FILTERS_KEY);
  } catch {
    // best-effort: private mode / quota / disabled storage
  }
}

// Whether this browser has already been offered the IP-derived opening scope. Its
// own key, NOT a value inside the filter set, because `saveJobFilters('')` removes
// JOB_FILTERS_KEY outright — so "storage is empty" cannot tell a browser that has
// never filtered from one that just cleared its filters, and a guess keyed on that
// would undo the clear on every subsequent visit.
//
// Only wiping browser storage re-arms the guess, which is the intended escape: at
// that point the browser is indistinguishable from a new one.
const GEO_SCOPE_KEY = 'hire.geoScopeOffered';

/** Whether the derived scope has already been offered — and therefore must not be
 *  offered again.
 *
 *  Unreachable storage reads as "offered", not "not yet". A marker that cannot be
 *  written is a guess that re-applies on every load and cannot be dismissed; losing
 *  the feature in private mode is the smaller failure. */
export function geoScopeOffered(): boolean {
  if (typeof localStorage === 'undefined') return true;
  try {
    return localStorage.getItem(GEO_SCOPE_KEY) !== null;
  } catch {
    return true;
  }
}

/** Record that the derived scope was offered, reporting whether the record stuck.
 *
 *  The return value is not decoration. Reads and writes fail independently — a
 *  quota-exhausted store answers `getItem` and throws on `setItem` — and in that
 *  state a caller that shrugged off the failure would offer the guess again on
 *  every visit, including to someone who had just cleared it. An unrecorded offer
 *  is one the visitor cannot dismiss, so it must not be made. */
export function markGeoScopeOffered(): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    localStorage.setItem(GEO_SCOPE_KEY, '1');
    return true;
  } catch {
    return false;
  }
}
