import { untrack } from 'svelte';
import { browser } from '$app/env';
import { replaceState } from '$app/navigation';
import { geoScopeOffered, loadJobFilters, markGeoScopeOffered } from '#lib/filterStorage.js';
import type { FilterStore } from '#lib/filters.js';
import { fetchRegion, geoScopeQuery, shouldOfferGeoScope, WORLDWIDE_REGION } from '#lib/geoScope.js';

/** JobsView's opening scope for a first-time visitor: their region (from the IP country)
 *  plus worldwide, offered once per browser and marked as inferred until they change it.
 *  Constructed during component initialisation, since it registers an effect. */
export class GeoScopeGuess {
  readonly #filters: FilterStore;
  readonly #beforeApply: () => void;

  // What `shouldOfferGeoScope` decides on, read fresh each call — the address bar and
  // storage both move under us. Client-only: `location` does not exist on the server.
  // Shared with `offerGeoScope`, which asks the same question again after its round
  // trip; two copies of these three keys would be two things to keep in step.
  #guards = () => ({
    search: location.search,
    storedFilters: loadJobFilters(),
    offered: geoScopeOffered(),
  });

  // Whether an opening scope is still to be guessed from the visitor's IP country, and
  // so whether the facet measurement below is worth making yet. Asked here, at setup,
  // because the answer is knowable without the network: `offerGeoScope` clears the same
  // guards before it fetches anything, and only a first-time visitor landing on a bare
  // URL gets past them. Everybody else — a repeat visitor, a link carrying filters, a
  // restored set — is false here and waits for nothing.
  //
  // What the wait buys: measured from Brazil on 2026-08-21, a cold feed spent 778ms on
  // a facet count of the unscoped catalogue that `/geo/region` (330ms) then replaced
  // wholesale, and the two overlapped, so the discarded one was also competing for the
  // connection the useful one needed. One scope, measured once.
  //
  // Never true on the server, where `browser` short-circuits before `location` is read.
  pending = $state(false);

  // The opening scope guessed from the visitor's IP country, once it has been
  // applied — null whenever the scope is theirs rather than ours. The header trigger
  // reads it to say the scope was inferred; see `inferred` below.
  guessedRegion = $state<string | null>(null);

  // The guess is only still ours while the geography is EXACTLY what we set: our
  // region plus worldwide, nothing excluded, no country or city of their own. Any
  // edit to the scope fails this and the marking drops, with no event to subscribe
  // to and no flag to remember to clear. Filters outside geography (seniority, a
  // search term) leave it standing — they did not touch the scope.
  readonly inferred = $derived.by(() => {
    if (this.guessedRegion === null) return false;
    const f = this.#filters.value;
    const untouched = (param: string) => {
      const st = f.facets[param];
      return !st || (st.include.length === 0 && st.exclude.length === 0);
    };
    const ours = [this.guessedRegion, WORLDWIDE_REGION];
    const regions = f.facets.regions;
    return (
      !!regions &&
      regions.exclude.length === 0 &&
      regions.include.length === ours.length &&
      ours.every((r) => regions.include.includes(r)) &&
      untouched('countries') &&
      untouched('metros') &&
      untouched('cities')
    );
  });

  // Open a first-time visitor on their own region plus worldwide. The precedence
  // itself lives in `shouldOfferGeoScope`, where it is a pure function and a test
  // can state each rule; this reads the browser and acts on the answer.
  //
  // The URL is written directly and re-read rather than going through
  // `filters.apply()`: apply() is an explicit write, and on the standalone list an
  // explicit write mirrors itself into `hire.jobFilters`. Storage records what the
  // visitor chose, and this is a guess. Writing the address bar and re-seeding from
  // it is the same path an ordinary navigation takes, which by construction persists
  // nothing. It is also safe by now — `replaceState` is not available during the
  // initial `enter`, but a network round trip has passed since.
  async offer() {
    // The page this offer is for. A request is in flight for a few hundred
    // milliseconds and the visitor can leave in that time — onto a job page, onto
    // /companies — and every one of those routes has an empty query string, so the
    // guards below would pass and the scope would land on a page it was never meant
    // for. The pathname captured here is the only thing that can tell those apart.
    const pathname = location.pathname;
    if (!shouldOfferGeoScope(this.#guards())) return;

    const region = await fetchRegion();
    // Nothing was offered, so nothing is marked: a deployment whose edge sends no
    // country would otherwise spend its one chance per browser on a null answer.
    if (!region) return;
    // Re-read the world, do not trust the snapshot: while we were asking they may
    // have navigated away, filtered, or been offered the scope by another mount.
    // Any of those outranks a guess; ours is dropped, not queued.
    if (location.pathname !== pathname || !shouldOfferGeoScope(this.#guards())) return;

    // A guess that cannot be recorded is a guess that re-applies on every visit and
    // cannot be dismissed, so a failed write means no scope at all.
    if (!markGeoScopeOffered()) return;
    // Keep what is on screen until the scoped page arrives, so the swap happens in
    // place instead of through a spinner the height of the whole list.
    this.#beforeApply();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query write to the current pathname; there is no route to resolve
    replaceState(`${pathname}?${geoScopeQuery(region)}`, {});
    this.#filters.syncFromUrl();
    // Claimed only now that the filters already carry the scope. Set before the
    // re-seed, it would sit for a tick beside geography that does not match it, and
    // the latch in the constructor would drop it before the trigger ever said anything.
    this.guessedRegion = region;
  }

  /** `beforeApply` runs just before the guessed scope is written, so the list can keep
   *  what is on screen until the scoped page arrives. */
  constructor(filters: FilterStore, standalone: boolean, beforeApply: () => void) {
    this.#filters = filters;
    this.#beforeApply = beforeApply;
    this.pending = browser && standalone && shouldOfferGeoScope(this.#guards());

    // Ours until the geography first stops matching, and not ours again after that.
    // Without the latch, someone who clears the guess and then picks the same region
    // by hand lands back in a state the check above cannot tell from the guess, and
    // the trigger would tell them the site inferred a scope they chose themselves.
    $effect(() => {
      if (this.guessedRegion !== null && !this.inferred) untrack(() => (this.guessedRegion = null));
    });
  }
}
