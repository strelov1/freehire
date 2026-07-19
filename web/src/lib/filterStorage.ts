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

// Set once this browser has made any explicit filter change — an edit, an applied
// saved search, or a clear. It's a separate key because clearing *removes*
// JOB_FILTERS_KEY, so the filters' absence alone can't tell a first-time visitor
// (offer the default) from someone who deliberately cleared (respect the empty set).
const FILTERS_TOUCHED_KEY = 'hire.jobFiltersTouched';

// The same "touched" marker, mirrored into a cookie so the SSR `load` can read it
// (localStorage is invisible to the server). It gates the first-visit default
// redirect — see firstVisit.ts. Kept in lockstep with FILTERS_TOUCHED_KEY by
// saveJobFilters below.
export const FILTERS_TOUCHED_COOKIE = 'hire_filters_touched';

/** The filter set a first-time visitor lands on: fully-remote roles open worldwide.
 *  Same facet vocabulary as the `remote-worldwide` collection (see collections.ts). */
export const DEFAULT_JOB_FILTERS = 'work_mode=remote&regions=global';

/** The stored filter query string, or '' when absent/unavailable. */
export function loadJobFilters(): string {
  if (typeof localStorage === 'undefined') return '';
  try {
    return localStorage.getItem(JOB_FILTERS_KEY) ?? '';
  } catch {
    return '';
  }
}

/** True once the visitor has changed filters at least once (including clearing them).
 *  Absent only for a browser that has never touched the /jobs filters — the single
 *  case offered the first-visit default. */
export function hasChangedFilters(): boolean {
  if (typeof localStorage === 'undefined') return false;
  try {
    return localStorage.getItem(FILTERS_TOUCHED_KEY) !== null;
  } catch {
    return false;
  }
}

/** Mirror the applied filters to storage. An empty string removes the key, so a
 *  cleared filter set leaves nothing to restore. Either way the change marks the
 *  browser as touched — in both localStorage and the server-readable cookie — so the
 *  first-visit default is never re-offered. */
export function saveJobFilters(qs: string): void {
  markFiltersTouchedCookie();
  if (typeof localStorage === 'undefined') return;
  try {
    if (qs) localStorage.setItem(JOB_FILTERS_KEY, qs);
    else localStorage.removeItem(JOB_FILTERS_KEY);
    localStorage.setItem(FILTERS_TOUCHED_KEY, '1');
  } catch {
    // best-effort: private mode / quota / disabled storage
  }
}

/** Set the server-readable "touched" cookie so the SSR first-visit redirect stops
 *  offering the default. One year, path=/, Lax; Secure on https so it isn't sent in
 *  the clear. Best-effort and browser-only (no document ⇒ SSR/test no-op). */
function markFiltersTouchedCookie(): void {
  if (typeof document === 'undefined') return;
  const secure = typeof location !== 'undefined' && location.protocol === 'https:' ? '; secure' : '';
  document.cookie = `${FILTERS_TOUCHED_COOKIE}=1; path=/; max-age=31536000; samesite=lax${secure}`;
}
