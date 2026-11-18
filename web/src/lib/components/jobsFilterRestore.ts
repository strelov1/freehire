import { afterNavigate } from '$app/navigation';
import { loadJobFilters } from '#lib/filterStorage.js';
import type { FilterStore } from '#lib/filters.js';
import type { GeoScopeGuess } from '#lib/geoScopeGuess.svelte.js';

// Re-seed the filters from the URL on every real navigation (initial load, the
// "Jobs" nav link, back/forward). On the standalone list a navigation that lands
// on a bare /jobs (empty URL) instead restores the last persisted filters, so an
// ordinary navigation never drops them ("Clear all" persisted an empty set, so it
// stays cleared).
//
// afterNavigate (not a URL-tracking $effect) because it runs only after the router
// is initialized: apply()'s replaceState is safe on a client-side navigation (a
// hydration-time $effect would throw "before router is initialized"), its `applied`
// change lands after the reload effect's first pass, and our own shallow replaceState
// writes don't re-fire it — so there's no restore loop. Restore is skipped on the
// initial `enter` navigation (a hard load / refresh): the router isn't ready for
// replaceState yet, and the SSR already rendered that exact URL, so we just re-seed
// from it. Every restore-worthy case — the "Jobs" nav link, cross-route entry,
// back/forward — is a client-side navigation, where it works. location.search is the
// address-bar truth (page.url can lag after shallow routing). The company-embedded
// list (persist off) keeps the plain re-seed — unchanged.
//
// Call during component initialisation, as afterNavigate requires.
export function restoreFiltersOnNavigate(filters: FilterStore, geo: GeoScopeGuess) {
  afterNavigate((nav) => {
    // SvelteKit 3 fires this for shallow routing too, i.e. for our own URL writes.
    if (nav.shallow) return;
    const stored = nav.type !== 'enter' && location.search === '' ? loadJobFilters() : '';
    if (stored) {
      filters.apply(stored);
      // A restored set outranks the guess, so nothing is going to be asked for and
      // the facet measurement must not keep waiting on an answer that is not coming.
      geo.pending = false;
    } else {
      filters.syncFromUrl();
      // Last in the precedence chain, and the only branch that runs on a cold
      // load: URL params were just applied, or there was nothing to restore.
      //
      // Released on EVERY outcome — scope applied, no region, guard failed on the
      // re-read, storage refused the marker — because the facet measurement is held
      // behind this and a path that forgot to release would leave the filter panel
      // permanently countless.
      void geo.offer().finally(() => (geo.pending = false));
    }
  });
}
