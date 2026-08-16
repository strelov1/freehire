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
