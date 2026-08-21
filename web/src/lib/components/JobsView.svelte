<script lang="ts">
  import { onMount, untrack, type Snippet } from 'svelte';
  import { Layers } from '@lucide/svelte';
  import { browser } from '$app/environment';
  import { afterNavigate, goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api, type Slice } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { profileStore } from '$lib/profile.svelte';
  import { computeClientMatch } from '$lib/jobMatch';
  import { ensureViewedLoaded } from '$lib/viewedJobs.svelte';
  import { ensureSavedLoaded } from '$lib/savedJobs.svelte';
  import { ensureDismissedLoaded, isDismissed, markUndismissed } from '$lib/dismissedJobs.svelte';
  import { latestOnly } from '$lib/latestOnly';
  import { Paginator } from '$lib/paginated.svelte';
  import { pageCount, pageOffset } from '$lib/pagination';
  import Pagination from './Pagination.svelte';
  import { FilterStore, filtersToParams, activeFilterCount } from '$lib/filters';
  import { geoScopeOffered, loadJobFilters, markGeoScopeOffered } from '$lib/filterStorage';
  import { geoScopeQuery, shouldOfferGeoScope, WORLDWIDE_REGION } from '$lib/geoScope';
  import {
    bannerVisible,
    loadOnboardingState,
    markDone,
    markSeen,
    narrowestFacet,
    type OnboardingLifecycle,
  } from '$lib/onboarding';
  import { consumePendingAlert } from '$lib/saveSearchAlert';
  import OnboardingWizard from './onboarding/OnboardingWizard.svelte';
  import OnboardingBanner from './onboarding/OnboardingBanner.svelte';
  import OnboardingAlertBanner from './onboarding/OnboardingAlertBanner.svelte';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { setListSearchTarget } from '$lib/listSearch.svelte';
  import { track } from '$lib/analytics';
  import type { Job, FacetCounts } from '$lib/types';
  import FilterSummary from './filters/FilterSummary.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import ListToolbar from './ListToolbar.svelte';
  import States from './States.svelte';
  import JobRow from './JobRow.svelte';
  import HiddenToast from './HiddenToast.svelte';

  // Filters live in the URL; the route `load` searches by them and returns the
  // first page as `initial`, so the rows are in the initial HTML for SSR/share/
  // reload. After hydration the view is client-driven: FilterStore writes the URL
  // synchronously and exposes a debounced `applied` snapshot; a filter change
  // reloads the list + counts client-side off `applied` (no navigation), and
  // infinite scroll pages the rest.
  //
  // `scope` pins extra search params that the user can't change (e.g. the company
  // page passes `{ company_slug }`): they're merged into every search but kept out
  // of `filters`/the URL, so they're not user-selectable facets. `excludeFacets`
  // hides facets that are redundant under that scope (e.g. Source on a company).
  // `sidebarTop` renders above the filter summary in the desktop sidebar (e.g. the
  // company page's facts card); the standalone /jobs list omits it.
  //
  // `initialParams` overrides the seed below with the exact param string the
  // route's `load` searched with (page.url.searchParams minus `page`, see
  // +page.server.ts): the client must seed off that same string, not the raw
  // URL, or the two would disagree and the mount effect would immediately
  // discard the SSR page and refetch.
  //
  // `currentPage` is the `?page=N` the route's `load` served, and it is REQUIRED:
  // page links are now the only way through the results, so a route that mounts
  // this without honouring `?page=N` would render a nav whose every link leads
  // back to the page already on screen. Required rather than optional so that
  // mistake is a type error instead of a listing that silently stops at twenty.
  let {
    initial,
    scope = {},
    excludeFacets = [],
    sidebarTop,
    initialParams,
    currentPage,
  }: {
    initial: Slice<Job>;
    scope?: Record<string, string>;
    excludeFacets?: string[];
    sidebarTop?: Snippet;
    initialParams?: string;
    currentPage: number;
  } = $props();

  // Standalone /jobs (no fixed scope) hands its text search to the header; an
  // embedded, scoped instance (e.g. a company page) keeps its own inline input.
  const standalone = $derived(Object.keys(scope).length === 0);

  // Seed filters from the current URL so the server and the hydrated client
  // render the same filtered view. Only the standalone list persists to storage;
  // the embedded company list must not clobber the shared key. Persistence is
  // fixed for the store's life, so the initial `standalone` and `initialParams`
  // are captured once.
  const seedParams = untrack(() =>
    initialParams != null ? new URLSearchParams(initialParams) : page.url.searchParams,
  );
  const filters = new FilterStore(seedParams, untrack(() => standalone));

  // The user's (debounced) facet filters plus the fixed `scope` params
  // (company_slug, …). Reads `applied` so typing doesn't fetch per keystroke.
  const scopedParams = () => {
    const p = filtersToParams(filters.applied);
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return p;
  };

  // The feed browses via keyword search, newest-first. Both take the same facet
  // params and return the same Job shape.
  const makePaginator = () =>
    new Paginator<Job>((limit, offset) => api.searchJobs(scopedParams(), limit, offset), {
      keyOf: (job) => job.public_slug,
    });

  // The page being read. Starts at the route's `?page=N` and survives a reload that
  // didn't change the query; a changed query resets it to 1, because the visitor is
  // now looking at a different result set (and FilterStore has already dropped
  // `page` from the URL — it only ever writes facet params).
  let activePage = $state(untrack(() => currentPage));

  // Seeded from the server-rendered page — an intentional one-time snapshot of the
  // props, which the effects below re-take when the page or the query changes.
  const seeded = makePaginator();
  untrack(() => seeded.seed(initial, pageOffset(currentPage)));
  let jobs = $state.raw(seeded);

  // The live facet distribution (value → count per facet), feeding the dynamic
  // selects (skills, countries) so the user sees which values exist and how many
  // jobs each has under the current filters. A failed fetch leaves the prior
  // counts — the selects degrade to plain (countless) options, never break.
  // latestOnly stops a slow earlier response overwriting a newer one.
  let counts = $state.raw<FacetCounts | null>(null);
  const refreshCounts = latestOnly(
    () => api.facetCounts(scopedParams()),
    (c) => (counts = c),
  );

  // The role distribution the header's suggestions are ranked and filtered by. Its own
  // fetch, deliberately: `counts` above is scoped by the text query, and a suggestion
  // list keyed off that would rank by "jobs matching what you have typed so far", lag
  // it by one debounce, and drop roles in and out mid-word. One facet, no `q` — the
  // rest of the filter scope stays, so the figure still answers what a click would
  // give. Refreshed only when a non-text filter changes; typing does not touch it.
  const roleScopeParams = () => {
    const p = scopedParams();
    p.delete('q');
    return p;
  };
  let roleCounts = $state.raw<FacetCounts | null>(null);
  const refreshRoleCounts = latestOnly(
    () => api.facetCounts(roleScopeParams(), { facets: ['role'] }),
    (c) => (roleCounts = c),
  );

  // Minimum profile-match slider: a client-only post-filter over the already-fetched
  // page, not a search facet — the match percent depends on the viewer's own profile
  // skills, which the backend/index never sees, so there's nothing to send server-side
  // (unlike salary/freshness, which narrow the actual query). Local and URL-free: a
  // shared link's match filter would mean nothing to whoever opens it. `null` means
  // "no threshold" (the slider's own default position doubles as "off").
  let minMatch = $state<number | null>(null);

  // Only a signed-in user with skills on their profile has a real match percent per
  // card (see resolveMatchState's `ready` state in JobRow) — everyone else sees a
  // teaser or nothing, so the slider would filter against a number that isn't real.
  $effect(() => {
    if (isAuthenticated()) profileStore.ensureLoaded();
  });
  const profileSkills = $derived(profileStore.profile?.skills ?? []);
  const matchFilterAvailable = $derived(isAuthenticated() && profileStore.loaded && profileSkills.length > 0);
  // Drop a stale threshold the moment the slider would disappear (sign-out, profile
  // cleared) so it can't silently keep hiding jobs with no control left to reset it.
  $effect(() => {
    if (!matchFilterAvailable) minMatch = null;
  });

  let modalOpen = $state(false);
  let started = false;
  // Signature of the applied filters last reported as a search, so a re-run that
  // didn't change the filters (a back/forward re-seed) doesn't emit a spurious
  // funnel event — and, since it also decides `sameQuery`, doesn't reset the page.
  //
  // Seeded with what the route searched with rather than left empty. Empty only
  // matches an unfiltered feed, so on `/?q=engineer&page=3` the first re-run read
  // as a brand-new search and snapped the reader from page 3 back to page 1 — and
  // logged a search they had not performed. It went unnoticed while scrolling was
  // how anyone paged; the page links made it visible.
  let lastSearchKey = untrack(() => filtersToParams(filters.applied).toString());

  // The filter scope the role distribution was last measured under, minus the text
  // query it deliberately ignores. `null` rather than '' because '' is a REAL key —
  // it is what an unfiltered feed serializes to, which is the commonest first paint
  // of all, and seeding with it made the first measurement look like a repeat and
  // never fire.
  let lastRoleScopeKey: string | null = null;

  // Onboarding: the one-time nudge banner + wizard, standalone-only. The lifecycle
  // lives in localStorage (client-only); seed it at init on the client so a returning
  // (dismissed/completed) visitor never flashes a banner before mount. The banner is
  // the sole entry — once dismissed or completed it retires; there is no persistent
  // re-open control.
  let wizardOpen = $state(false);
  // Starts 'unseen' on both sides so the hydrated markup matches the server's; the
  // stored value arrives on mount, by which point app.css has already hidden the
  // banner for anyone who dismissed or completed it. Reading localStorage here
  // instead would make the client disagree with the SSR output on the very first
  // frame. Same shape as ProductHuntBanner's `dismissed`.
  let onboardingState = $state<OnboardingLifecycle>('unseen');
  // The ephemeral post-onboarding Telegram-alert offer (set after the wizard, or on
  // mount to resume a pending alert after sign-in). Dismissible; not persisted.
  let alertBanner = $state<{ query: string; autostart: boolean } | null>(null);
  // Show only to an un-nudged visitor with no active facet filters AND no text query,
  // so a shared search/filter link is never interrupted (activeFilterCount ignores the
  // query, so check it explicitly).
  //
  // Deliberately NOT gated on `browser`. It was, and that is what made this banner
  // the site's largest source of layout shift: the feed rendered without it, then
  // hydration inserted 74px directly above the rows. Lab Lighthouse missed it
  // (0.066) because it never carries a returning visitor's localStorage; CrUX put
  // the mobile home page at CLS 0.28. It is server-rendered now, and hidden before
  // first paint by app.css for a visitor who has already seen it.
  const showBanner = $derived(
    standalone &&
      bannerVisible(onboardingState, filters.active > 0 || filters.value.q.trim() !== ''),
  );

  function dismissBanner() {
    markSeen();
    onboardingState = 'seen';
  }
  function cancelWizard() {
    wizardOpen = false;
    markSeen();
    onboardingState = loadOnboardingState(); // markSeen never downgrades a completed run
  }
  function completeWizard(query: string) {
    // Apply through the same store path as a saved search — feed, counts, and
    // localStorage all reconfigure via the existing effect; then the banner retires.
    filters.apply(query);
    markDone();
    onboardingState = 'done';
    wizardOpen = false;
    // Peak intent: offer to keep this feed as a Telegram alert.
    alertBanner = { query, autostart: false };
  }
  // Narrow-feed relief: the single narrowest applied facet to drop (skills → regions →
  // seniority; never the role), or null if none. Shared by the empty-state guard and
  // the relax action so they can't disagree. Never broadens on its own.
  const relaxTarget = $derived(narrowestFacet(filters.value));
  function relaxFeed() {
    if (relaxTarget) filters.clearFacet(relaxTarget);
  }

  // Resume a save the user started before signing in. Reactive on the session so it
  // fires whether auth resolved server-side (an OAuth full-page redirect — mount is
  // already authed) or flipped in-tab (an in-dialog sign-in re-resolves page.data.user
  // without a remount). Consumed exactly once; standalone list only; skipped if a banner
  // is already showing.
  $effect(() => {
    if (!untrack(() => standalone) || alertBanner) return; // effect: client-only, no browser guard
    if (!isAuthenticated()) return;
    const pending = consumePendingAlert();
    if (pending !== null) alertBanner = { query: pending, autostart: true };
  });

  // Live disjunctive facet counts for the staged filter set (built by the modal),
  // merged with the fixed scope params (e.g. company_slug) so every control's counts —
  // and the "Show N jobs" total — match the list. Disjunctive so a selected facet
  // still shows its siblings' counts.
  const stagedCounts = (params: URLSearchParams) => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- transient: built, mutated, passed to the API once; not reactive state
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return api.facetCounts(p, { disjunctive: true });
  };

  // The server-rendered `initial` was searched for `page.url`. After a back/forward
  // onto a shallow-routing entry that lags page.url behind the address bar, `initial`
  // is stale (it holds the pre-filter page) while the filters seed from the real URL.
  // Detect that mismatch so the first effect run reloads instead of keeping `initial`.
  const initialStale = browser && page.url.search !== location.search;

  // For a signed-in user, load the set of already-viewed and already-saved slugs so
  // JobRow can dim seen cards and fill saved bookmarks (no-op when signed out — the
  // sets stay empty). Cleanup: cancel any pending debounced reload so it can't fire
  // after this view is gone.
  onMount(() => {
    // Catch up with the stored lifecycle now that localStorage is readable. For a
    // returning visitor this drops the server-rendered banner — with no shift,
    // because app.css already took it out of the flow before first paint.
    onboardingState = loadOnboardingState();
    if (isAuthenticated()) {
      ensureViewedLoaded();
      ensureSavedLoaded();
      ensureDismissedLoaded();
    }
    // Register this page's store so the header search drives it. This holds for the
    // standalone /jobs list AND the company page's embedded, scoped list — on
    // /companies/:slug the header search filters that company's postings (there's
    // no inline box), so both modes route their text search through the header.
    // The adapter also exposes `filterScope` (the store + a reactive counts getter)
    // so the header renders its Location & work-format popover on these jobs-backed
    // lists; the company list registers a bare store with no filterScope, so the
    // popover stays hidden there.
    setListSearchTarget({
      get value() {
        return filters.value;
      },
      setQuery: (q) => filters.setQuery(q),
      filterScope: { store: filters, counts: () => counts, variant: 'jobs', inferred: () => scopeInferred },
      roleSuggest: {
        counts: () => roleCounts,
        active: () => filters.facet('role').include,
        apply: (slug) => {
          // Its own event, not a flag on `search`: the question this answers is how
          // often the dropdown is what puts the role facet on, and the role facet
          // measured 1.1% of searches before it existed.
          track('role_suggestion', { role: slug });
          filters.applyRole(slug);
        },
      },
      openFilters: () => (modalOpen = true),
      activeFilters: () => filters.active,
    });
    return () => {
      setListSearchTarget(null);
      filters.dispose();
    };
  });

  function reloadList(offset = 0) {
    const next = makePaginator();
    next.start(offset);
    jobs = next;
  }

  // Rows kept on screen while the guessed opening scope reloads the feed underneath
  // them. A filter change builds a fresh paginator, which starts empty and `loading`
  // — for a change the visitor made that is the honest answer, but the guess is one
  // NOBODY asked for, landing a few hundred milliseconds into the first visit. Left
  // alone it collapses the whole list to a spinner and re-expands it: a layout shift
  // measured against the page, attributed in full because it happens before any input.
  //
  // Only the guess holds rows over. Every other reload keeps its loading state.
  let holdover = $state.raw<Job[]>([]);
  const holdingOver = $derived(jobs.status === 'loading' && holdover.length > 0);
  // Released when the replacement is on screen. `holdover` is read through untrack
  // deliberately: as a tracked read it made this effect a dependency of its own
  // write, so capturing the rows re-ran it while the outgoing paginator was still
  // `ready` and it cleared them in the same tick — the hold never survived to the
  // swap it existed for.
  $effect(() => {
    if (jobs.status === 'loading') return;
    untrack(() => {
      if (holdover.length > 0) holdover = [];
    });
  });

  // Every read of "the rows on screen" goes through this, so the held-over rows and
  // the paginator's own cannot disagree — the empty state reading jobs.items directly
  // is what put "No matching jobs." over a list that was merely reloading.
  const displayItems = $derived(holdingOver ? holdover : jobs.items);

  // The feed minus the signed-in user's hidden jobs, and (when the match slider is
  // active) minus jobs below the chosen match threshold. Dismissal is cross-referenced
  // client-side against the shared dismissed set (loaded on mount), mirroring the
  // viewed/saved sets — the server search is untouched. Re-derives when the page
  // changes, when a hide/undo mutates the dismissed set, or when the slider moves, so
  // a hidden or filtered-out card drops (and an undone one returns) instantly. A job
  // with no skills has no percent to test (see computeClientMatch) and stays, matching
  // the card's own `no-skills` state, which shows no match at all rather than a false 0%.
  const visibleJobs = $derived(
    displayItems.filter((j) => {
      if (isDismissed(j.public_slug)) return false;
      if (matchFilterAvailable && minMatch != null && (j.skills ?? []).length > 0) {
        return computeClientMatch(j.skills ?? [], profileSkills).percent >= minMatch;
      }
      return true;
    }),
  );

  // `jobs.total` is the server's raw count for the query — accurate for every facet,
  // which all narrow the actual search, but not for the match slider, which only
  // trims the already-fetched page client-side. Showing the raw total next to a
  // shrunk list would read as a lie ("500 open jobs" over three visible cards), so
  // swap in what's actually on screen while the threshold is active.
  const listTotal = $derived(matchFilterAvailable && minMatch != null ? visibleJobs.length : jobs.total);

  // The pending "Job hidden — Undo" toast, or null. Set when a card is hidden; the
  // toast owns its auto-dismiss. Undo clears the slug's hidden mark (card returns via
  // visibleJobs) and confirms with the server; a failed undo is swallowed — the
  // durable recovery path is Activity → Hidden.
  let hiddenToast = $state<{ slug: string } | null>(null);

  function onHide(slug: string) {
    hiddenToast = { slug };
  }

  async function undoHide() {
    const pending = hiddenToast;
    if (!pending) return;
    markUndismissed(pending.slug);
    hiddenToast = null;
    try {
      await api.undismissJob(pending.slug);
    } catch {
      // Swallow: the job is already back in the feed optimistically, and Activity →
      // Hidden remains the durable way to manage hidden jobs.
    }
  }

  // Enter swipe mode carrying the current filters + query (same param shape the
  // list uses), so the deck reflects exactly what's on screen. `scope` params are
  // fixed context (e.g. company_slug) and go along too.
  function openSwipe() {
    const params = scopedParams();
    const qs = params.toString();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended query string
    goto(resolve('/jobs/swipe') + (qs ? `?${qs}` : ''));
  }

  // Reload list + counts whenever the debounced filters change — a settled
  // keystroke, an immediate facet toggle, or a back/forward re-seed. Skip the
  // first run for the list (the SSR `initial` already seeded page one); still
  // fetch counts on mount since they aren't server-rendered into this view.
  $effect(() => {
    void filters.applied; // track the debounced snapshot
    untrack(() => {
      refreshCounts();
      // The role distribution ignores the text query, so refetch it only when the rest
      // of the scope moves — otherwise every settled keystroke would spend a request
      // re-measuring something that did not change.
      const roleScopeKey = roleScopeParams().toString();
      if (roleScopeKey !== lastRoleScopeKey) {
        lastRoleScopeKey = roleScopeKey;
        refreshRoleCounts();
      }
      const firstRun = !started;
      if (firstRun) {
        started = true;
        // Keep the SSR `initial` page unless it was loaded for a different URL than
        // the address bar (stale shallow-routing restore) — otherwise `initial`
        // already matches the URL here, so no forced reload is needed.
        if (!initialStale) return;
      }
      // Funnel search — only when the applied filters actually changed: not the
      // initial paint or a back/forward re-seed to the same set.
      const searchKey = filtersToParams(filters.applied).toString();
      if (!firstRun && searchKey !== lastSearchKey) {
        track('search', {
          q: filters.applied.q.trim(),
          facets: activeFilterCount(filters.applied),
        });
      }
      // Same query: this is a re-seed (initial navigation, back/forward), not a new
      // search — reload the page being read instead of snapping back to the first.
      const sameQuery = searchKey === lastSearchKey;
      lastSearchKey = searchKey;
      if (!sameQuery) activePage = 1;
      reloadList(sameQuery ? pageOffset(activePage) : 0);
    });
  });

  // Following a page link is a real navigation, but SvelteKit reuses this component
  // across `?page=N` rather than remounting it — so `initial` and `currentPage` are
  // re-supplied while the state seeded from them is not. Without this the address bar
  // said page 4 and the feed still showed page 3, which is the whole nav being
  // decorative. It re-seeds from the `initial` the route just loaded rather than
  // fetching: the server already searched for exactly this page.
  $effect(() => {
    const nextPage = currentPage;
    const slice = initial;
    untrack(() => {
      if (nextPage === activePage) return;
      activePage = nextPage;
      const next = makePaginator();
      next.seed(slice, pageOffset(nextPage));
      jobs = next;
    });
  });

  // The opening scope guessed from the visitor's IP country, once it has been
  // applied — null whenever the scope is theirs rather than ours. The header trigger
  // reads it to say the scope was inferred; see `scopeInferred` below.
  let guessedRegion = $state<string | null>(null);

  // The guess is only still ours while the geography is EXACTLY what we set: our
  // region plus worldwide, nothing excluded, no country or city of their own. Any
  // edit to the scope fails this and the marking drops, with no event to subscribe
  // to and no flag to remember to clear. Filters outside geography (seniority, a
  // search term) leave it standing — they did not touch the scope.
  const scopeInferred = $derived.by(() => {
    if (guessedRegion === null) return false;
    const f = filters.value;
    const untouched = (param: string) => {
      const st = f.facets[param];
      return !st || (st.include.length === 0 && st.exclude.length === 0);
    };
    const ours = [guessedRegion, WORLDWIDE_REGION];
    const regions = f.facets.regions;
    return (
      !!regions &&
      regions.exclude.length === 0 &&
      regions.include.length === ours.length &&
      ours.every((r) => regions.include.includes(r)) &&
      untouched('countries') &&
      untouched('cities')
    );
  });

  // Ours until the geography first stops matching, and not ours again after that.
  // Without the latch, someone who clears the guess and then picks the same region
  // by hand lands back in a state the check above cannot tell from the guess, and
  // the trigger would tell them the site inferred a scope they chose themselves.
  $effect(() => {
    if (guessedRegion !== null && !scopeInferred) untrack(() => (guessedRegion = null));
  });

  /** The visitor's region from the edge, or null for anyone the edge cannot place —
   *  a crawler, a missing header, a country outside the grouping. A failed request
   *  is the same answer: this is an opening convenience, never a thing to retry. */
  async function fetchRegion(): Promise<string | null> {
    try {
      const res = await fetch('/geo/region');
      if (!res.ok) return null;
      return ((await res.json()) as { region: string | null }).region;
    } catch {
      return null;
    }
  }

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
  async function offerGeoScope() {
    // The page this offer is for. A request is in flight for a few hundred
    // milliseconds and the visitor can leave in that time — onto a job page, onto
    // /companies — and every one of those routes has an empty query string, so the
    // guards below would pass and the scope would land on a page it was never meant
    // for. The pathname captured here is the only thing that can tell those apart.
    const pathname = location.pathname;
    const guards = () => ({
      search: location.search,
      storedFilters: loadJobFilters(),
      offered: geoScopeOffered(),
    });
    if (!shouldOfferGeoScope(guards())) return;

    const region = await fetchRegion();
    // Nothing was offered, so nothing is marked: a deployment whose edge sends no
    // country would otherwise spend its one chance per browser on a null answer.
    if (!region) return;
    // Re-read the world, do not trust the snapshot: while we were asking they may
    // have navigated away, filtered, or been offered the scope by another mount.
    // Any of those outranks a guess; ours is dropped, not queued.
    if (location.pathname !== pathname || !shouldOfferGeoScope(guards())) return;

    // A guess that cannot be recorded is a guess that re-applies on every visit and
    // cannot be dismissed, so a failed write means no scope at all.
    if (!markGeoScopeOffered()) return;
    // Keep what is on screen until the scoped page arrives, so the swap happens in
    // place instead of through a spinner the height of the whole list.
    holdover = jobs.items;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query write to the current pathname; there is no route to resolve
    replaceState(`${pathname}?${geoScopeQuery(region)}`, {});
    filters.syncFromUrl();
    // Claimed only now that the filters already carry the scope. Set before the
    // re-seed, it would sit for a tick beside geography that does not match it, and
    // the latch below would drop it before the trigger ever said anything.
    guessedRegion = region;
  }

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
  if (untrack(() => standalone)) {
    afterNavigate((nav) => {
      const stored = nav.type !== 'enter' && location.search === '' ? loadJobFilters() : '';
      if (stored) filters.apply(stored);
      else {
        filters.syncFromUrl();
        // Last in the precedence chain, and the only branch that runs on a cold
        // load: URL params were just applied, or there was nothing to restore.
        void offerGeoScope();
      }
    });
  } else {
    syncOnNavigation(filters);
  }
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 flex max-h-[calc(100vh-5rem)] flex-col gap-4 overflow-y-auto">
      {#if !standalone && jobs.status === 'ready'}
        <!-- Company view: the (filtered) open-job count as the sidebar's lead stat.
             The inline count above the list is hidden on desktop (shown only on
             mobile, where there's no sidebar), so it lives here instead. -->
        <div class="rounded-xl border border-border bg-card px-4 py-3">
          <p class="text-3xl font-semibold leading-none tracking-tight tabular-nums">
            {listTotal.toLocaleString()}
          </p>
          <p class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {listTotal === 1 ? 'open job' : 'open jobs'}
          </p>
        </div>
      {/if}
      {@render sidebarTop?.()}
      <div class="rounded-xl border border-border bg-card p-4">
        <FilterSummary store={filters} exclude={excludeFacets} onOpen={() => (modalOpen = true)} canSave={standalone} />
      </div>
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <ListToolbar
      total={displayItems.length > 0 ? listTotal : null}
      unit={listTotal === 1 ? 'job' : 'jobs'}
      onSwipe={standalone ? openSwipe : undefined}
      showDesktopTotal={standalone}
    />

    <!-- Onboarding nudges sit UNDER the toolbar so the feed controls stay at the top;
         each shows once (until dismissed or completed), then retires. Never blocks
         the feed below. -->
    {#if showBanner || alertBanner}
      <div class="mt-3">
        {#if showBanner}
          <OnboardingBanner onOpen={() => (wizardOpen = true)} onDismiss={dismissBanner} />
        {/if}
        {#if alertBanner}
          <OnboardingAlertBanner
            query={alertBanner.query}
            autostart={alertBanner.autostart}
            onDismiss={() => (alertBanner = null)}
          />
        {/if}
      </div>
    {/if}

    {#if jobs.status === 'loading' && !holdingOver}
      <States state="loading" />
    {:else if jobs.status === 'error'}
      <States state="error" message="Failed to load jobs." />
    {:else if displayItems.length === 0}
      <States state="empty" message="No matching jobs." />
      {#if standalone && relaxTarget}
        <!-- No semantic fallback in this slice: offer an honest one-step broaden
             instead of silently widening the feed. -->
        <div class="mt-4 flex justify-center">
          <button
            type="button"
            onclick={relaxFeed}
            class="inline-flex items-center gap-1.5 rounded-lg bg-brand px-4 py-2 text-sm font-semibold text-brand-foreground transition-opacity hover:opacity-90"
          >
            Broaden search
          </button>
        </div>
      {/if}
    {:else}
      {#if visibleJobs.length === 0}
        <!-- The search DID match on this page — the client-side post-filters removed
             every row. Saying "no matching jobs" here would blame the search for the
             reader's own settings, and the two causes are worth telling apart because
             one of them is a slider they can move. The paginator below still renders,
             so this is somewhere to leave rather than a dead end. -->
        <States
          state="empty"
          message={minMatch === null
            ? 'Every job on this page is one you hid.'
            : 'Every job on this page is hidden or below your match threshold.'}
        />
      {:else}
        <div class="flex flex-col gap-3">
          {#each visibleJobs as job (job.public_slug)}
            <JobRow {job} {onHide} />
          {/each}
        </div>
      {/if}

      <!-- Page links, and the only way through the results: a scroll-to-bottom
           auto-load used to sit here, which grew the page every time the reader
           neared the end of it and put the footer permanently out of reach.
           `pageCount` caps at the deepest page the search API will serve, so no
           link here walks into its "pagination too deep" 400. -->
      <Pagination
        current={activePage}
        total={pageCount(jobs.total)}
        pathname={page.url.pathname}
        params={filters.params}
      />
    {/if}
  </div>
</div>

{#if standalone}
  <!-- Desktop swipe-mode entry: an icon-only button pinned to the right viewport edge
       (mobile uses the inline toolbar / scroll-revealed tab, so this is md-only). Fixed,
       so it exists only while the standalone list is mounted and stays reachable while
       scrolling; kept below the z-40 mobile overlays. -->
  <button
    type="button"
    onclick={openSwipe}
    aria-label="Swipe mode"
    title="Swipe mode"
    class="fixed right-0 top-16 z-30 hidden items-center rounded-l-xl border border-r-0 border-border bg-secondary py-2.5 pl-2.5 pr-2 text-secondary-foreground shadow-md transition-colors hover:bg-accent md:flex"
  >
    <Layers class="size-4 shrink-0" />
  </button>
{/if}

<!-- Undo affordance for the hide gesture. Keyed by slug so hiding a second job while
     a toast is up restarts the countdown for the newer one. A hide-then-forget is
     still recoverable in Activity → Hidden. -->
{#if hiddenToast}
  {#key hiddenToast.slug}
    <HiddenToast onUndo={undoHide} onClose={() => (hiddenToast = null)} />
  {/key}
{/if}

<FilterModal
  store={filters}
  {counts}
  exclude={excludeFacets}
  savedSearches={standalone}
  open={modalOpen}
  onClose={() => (modalOpen = false)}
  {stagedCounts}
  matchAvailable={matchFilterAvailable}
  {minMatch}
  onMinMatchChange={(v) => (minMatch = v)}
/>

{#if standalone}
  <OnboardingWizard open={wizardOpen} {counts} onComplete={completeWizard} onCancel={cancelWizard} />
{/if}
