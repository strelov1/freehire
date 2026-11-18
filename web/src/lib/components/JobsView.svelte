<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { browser } from '$app/env';
  import { goto, replaceState } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '#lib/api.js';
  import { currentUser, isAuthenticated } from '#lib/auth.svelte.js';
  import { profileStore } from '#lib/profile.svelte.js';
  import { PUBLIC_OPEN_WITHIN } from '$app/env/public';
  import { openWithinEnabled } from '#lib/features.js';
  import { ensureViewedLoaded } from '#lib/viewedJobs.svelte.js';
  import { ensureSavedLoaded } from '#lib/savedJobs.svelte.js';
  import { ensureAppliedCompaniesLoaded } from '#lib/appliedCompanies.svelte.js';
  import { ensureDismissedLoaded, isDismissed } from '#lib/dismissedJobs.svelte.js';
  import { Paginator } from '#lib/paginated.svelte.js';
  import { pageCount, pageOffset, resumePage } from '#lib/pagination.js';
  import Pagination from './Pagination.svelte';
  import JobsEmptyState from './JobsEmptyState.svelte';
  import {
    FilterStore,
    filtersToParams,
    matchSortNeedsSkills,
    selectedSortFor,
    activeFilterCount,
    displayQuery,
  } from '#lib/filters.js';
  import JobListControls from './JobListControls.svelte';
  import JobsMapButtons from './JobsMapButtons.svelte';
  import JobsSidebar from './JobsSidebar.svelte';
  import JobsFeedNotices from './JobsFeedNotices.svelte';
  import JobsFeedList from './JobsFeedList.svelte';
  import { restoreFiltersOnNavigate } from './jobsFilterRestore';
  import { GeoScopeGuess } from '#lib/geoScopeGuess.svelte.js';
  import { jobsSearchTarget } from '#lib/jobsSearchTarget.js';
  import { narrowestFacet } from '#lib/onboarding.js';
  import { OnboardingNudge } from '#lib/onboardingNudge.svelte.js';
  import { syncOnNavigation } from '#lib/urlSynced.svelte.js';
  import { setListSearchTarget } from '#lib/listSearch.svelte.js';
  import { track } from '#lib/analytics.js';
  import type { Job } from '#lib/types.js';
  import FilterModal from './filters/FilterModal.svelte';
  import ListToolbar from './ListToolbar.svelte';
  import States from './States.svelte';
  import JobSearchError from './JobSearchError.svelte';
  import JobRowSkeleton from './JobRowSkeleton.svelte';
  import HiddenToast from './HiddenToast.svelte';
  import type { JobsViewProps } from './JobsView.props';
  import { clientMatchesOf, visibleFeed } from './feedVisibility';
  import { FeedCounts } from './feedCounts.svelte';
  import { HiddenJobUndo } from './hiddenJobUndo.svelte';

  let { initial, scope = {}, excludeFacets = [], sidebarTop, initialParams, currentPage }: JobsViewProps = $props();

  // Standalone /jobs (no fixed scope) hands its text search to the header; an
  // embedded, scoped instance (e.g. a company page) keeps its own inline input.
  const standalone = $derived(Object.keys(scope).length === 0);

  // Seed filters from the current URL so the server and the hydrated client
  // render the same filtered view. Only the standalone list persists to storage;
  // the embedded company list must not clobber the shared key. Persistence is
  // fixed for the store's life, so the initial `standalone` and `initialParams`
  // are captured once.
  const seedParams = untrack(() =>
    initialParams != null ? new URLSearchParams(initialParams) : new URLSearchParams(page.url.search),
  );
  const filters = new FilterStore(
    seedParams,
    untrack(() => standalone),
  );

  // The user's (debounced) facet filters plus the fixed `scope` params
  // (company_slug, …). Reads `applied` so typing doesn't fetch per keystroke.
  const scopedParams = () => {
    const p = filtersToParams(filters.applied);
    // Scoped loaders render anonymously and offer no control for this preference.
    if (!standalone) {
      p.delete('hide_seen');
      p.delete('former_employers');
      p.delete('watchlist');
    }
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
  //
  // On the client it starts from the address bar instead when the route loaded some
  // other URL — a back/forward onto a shallow-routed entry, see resumePage.
  let activePage = $state(
    untrack(() => (browser ? resumePage(page.url.search, location.search, currentPage) : currentPage)),
  );

  // Seeded from the server-rendered page — an intentional one-time snapshot of the
  // props, which the effects below re-take when the page or the query changes.
  const seeded = makePaginator();
  untrack(() => seeded.seed(initial, pageOffset(currentPage)));
  let jobs = $state.raw(seeded);

  // The result set on screen, minus the page being read: what ListToolbar keys its
  // count announcement on, so turning a page does not re-announce the same total.
  const resultSetKey = $derived(filtersToParams(filters.applied).toString());

  // The opening scope guessed for a first-time visitor (see GeoScopeGuess). Its `pending`
  // holds the facet measurement until the guess has been made or ruled out.
  const geo = new GeoScopeGuess(
    filters,
    untrack(() => standalone),
    () => (holdover = jobs.items),
  );

  // The live facet distribution (see FeedCounts).
  const feedCounts = new FeedCounts(scopedParams);
  // Facet counts cannot apply the former-employer constraint.
  const counts = $derived(filters.value.formerEmployers || filters.value.watchlist ? null : feedCounts.catalog);
  // The map dialog reads and writes `filters` itself, so a click on the map narrows the
  // list behind it. Its module (the land outline among it) loads on the first open only.
  let mapOpen = $state(false);
  let mapLoaded = $state(false);

  // Minimum profile-match slider: a client-only post-filter over the already-fetched
  // page — the match percent depends on the viewer's own profile skills, which the
  // backend/index never sees. The threshold itself is a JobFilters field so a saved
  // search carries it; `minMatch` below is what it means for THIS viewer.

  // Only a signed-in user with skills on their profile has a real match percent per
  // card (see resolveMatchState's `ready` state in JobRow) — everyone else sees a
  // teaser or nothing, so the slider would filter against a number that isn't real.
  $effect(() => {
    if (isAuthenticated()) profileStore.ensureLoaded();
  });
  const profileSkills = $derived(profileStore.profile?.skills ?? []);
  // The server reads former_employers only for a signed-in caller, so only then does it
  // explain an empty list.
  const formerEmployersApplied = $derived(standalone && isAuthenticated() && filters.applied.formerEmployers);
  const watchlistApplied = $derived(standalone && isAuthenticated() && filters.applied.watchlist);
  const matchFilterAvailable = $derived(isAuthenticated() && profileStore.loaded && profileSkills.length > 0);
  // Ignored the moment the slider would disappear (sign-out, profile cleared, or a
  // shared link opened by someone without a profile) so it can't silently keep hiding
  // jobs with no control left to reset it.
  const minMatch = $derived(matchFilterAvailable ? filters.applied.minSkillMatch : null);

  // The orderings this caller can choose between, and which one the control shows —
  // both pure, both in facetModel so they can be tested (see sortOptionsFor).
  //
  const selectedSort = $derived(selectedSortFor(filters.value));

  // Whether that notice is showing: match is the ordering in force and there are no
  // skills to rank against, so the server has quietly served newest instead.
  //
  // Three states, not two — "has skills", "has none", and "we do not know yet".
  // `matchFilterAvailable` folds the third into the second, and the profile arrives one
  // round trip after the page does, so asking it alone would put this notice in the
  // server-rendered HTML for a signed-in user WITH skills and take it away a moment
  // later. `loaded` exists for exactly this (see userResource.svelte.ts). Signed out
  // there is nothing to wait for: no account means no profile, and that is known at once.
  const skillsSettled = $derived(!isAuthenticated() || profileStore.loaded);
  const matchNeedsSkills = $derived(skillsSettled && matchSortNeedsSkills(filters.value, matchFilterAvailable));

  // Under best match the server applies the threshold across the whole ranking, so the
  // page is used as served; under any other ordering the browser still trims the page.
  const serverMatch = $derived(minMatch != null && selectedSort === 'match');
  const matchSortHint = $derived(minMatch != null && selectedSort !== 'match');

  // Which date the above-list select bounds. Not $derived: the flag is a runtime env
  // value read once at module load, and it cannot change while the page is mounted.
  const openWithinAvailable = openWithinEnabled({ PUBLIC_OPEN_WITHIN });

  let modalOpen = $state(false);
  let started = false;
  // Signature of the applied filters last reported as a search, so a re-run that
  // didn't change the filters (a back/forward re-seed) doesn't emit a spurious
  // funnel event — and, since it also decides `sameQuery`, doesn't reset the page.
  //
  // Seeded with what the route searched with rather than left empty. Empty only
  // matches an unfiltered feed, so on `/jobs?q=engineer&page=3` the first re-run read
  // as a brand-new search and snapped the reader from page 3 back to page 1 — and
  // logged a search they had not performed. It went unnoticed while scrolling was
  // how anyone paged; the page links made it visible.
  let lastSearchKey = untrack(() => filtersToParams(filters.applied).toString());

  // The onboarding nudge and the alert offer after it (see OnboardingNudge).
  const nudge = new OnboardingNudge(
    untrack(() => standalone),
    () => filters.active > 0 || filters.value.q.trim() !== '',
  );
  // Narrow-feed relief: the single narrowest applied facet to drop (skills → regions →
  // seniority; never the role), or null if none. Shared by the empty-state guard and
  // the relax action so they can't disagree. Never broadens on its own.
  const relaxTarget = $derived(narrowestFacet(filters.value));
  function relaxFeed() {
    if (relaxTarget) filters.clearFacet(relaxTarget);
  }

  const stagedCounts = (params: URLSearchParams) => feedCounts.staged(params, scope);

  // The server-rendered `initial` was searched for `page.url`. After a back/forward
  // onto a shallow-routing entry that lags page.url behind the address bar, `initial`
  // is stale (it holds the pre-filter page) while the filters seed from the real URL.
  // Detect that mismatch so the first effect run reloads instead of keeping `initial`.
  const initialStale = browser && page.url.search !== location.search;

  // For a signed-in user, load the set of already-viewed and already-saved slugs (and the
  // employers already applied to) so JobRow can dim seen cards, fill saved bookmarks and
  // mark applied-to companies (no-op when signed out — the
  // sets stay empty). Cleanup: cancel any pending debounced reload so it can't fire
  // after this view is gone.
  onMount(() => {
    // Catch up with the stored lifecycle now that localStorage is readable. For a
    // returning visitor this drops the server-rendered banner — with no shift,
    // because app.css already took it out of the flow before first paint.
    nudge.load();
    if (isAuthenticated()) {
      ensureViewedLoaded();
      ensureSavedLoaded();
      ensureDismissedLoaded();
      ensureAppliedCompaniesLoaded();
    }
    // Register this page's store so the header search drives it (see jobsSearchTarget).
    setListSearchTarget(
      jobsSearchTarget(filters, {
        counts: () => counts,
        inferred: () => geo.inferred,
        openFilters: () => (modalOpen = true),
      }),
    );
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

  function retrySearch() {
    feedCounts.refresh();
    if (jobs.status === 'error') reloadList(pageOffset(activePage));
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

  // What the feed shows of the fetched page, and the total beside it (see feedVisibility.ts).
  const clientMatches = $derived(clientMatchesOf(displayItems, profileSkills));
  // The threshold the browser applies itself: only off best match, where the server does.
  const clientMinMatch = $derived(!serverMatch && matchFilterAvailable ? minMatch : null);
  const visibleJobs = $derived(visibleFeed(displayItems, clientMatches, clientMinMatch, isDismissed));
  const listTotal = $derived(clientMinMatch != null ? visibleJobs.length : jobs.total);

  // The "Job hidden — Undo" toast (see HiddenJobUndo).
  const hidden = new HiddenJobUndo();

  // Enter swipe mode carrying the current filters + query (same param shape the
  // list uses), so the deck reflects exactly what's on screen. `scope` params are
  // fixed context (e.g. company_slug) and go along too.
  function openSwipe() {
    const params = scopedParams();
    const qs = params.toString();
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() applied to the path; the rule can't see through the appended query string
    goto(resolve('/jobs/swipe') + (qs ? `?${qs}` : ''));
  }

  // Measure the facets for the scope now in force — on mount too, since they are not
  // server-rendered into this view.
  //
  // Its own effect, apart from the list reload below, because the two answer to
  // different triggers: releasing the geography hold must re-measure the facets and must
  // NOT reload the list. In one effect that did both, the visitor whose guess resolved
  // to no region got a second, identical search — the release re-ran the whole body
  // while the filters had not moved.
  $effect(() => {
    void filters.applied; // track the debounced snapshot
    // Read outside untrack: clearing the hold is what re-runs this, and it is the only
    // thing that ever will for a visitor whose guess came back empty.
    if (geo.pending) return;
    untrack(() => {
      feedCounts.refresh();
    });
  });

  // Personal feed preferences must be re-applied when the reader changes in-tab.
  const searchUserID = $derived(
    standalone &&
      (filters.applied.hideSeen || filters.applied.formerEmployers || filters.applied.watchlist || serverMatch)
      ? (currentUser()?.id ?? null)
      : null,
  );

  let lastSearchUserID = untrack(() => searchUserID);

  // Reload the list whenever the debounced filters change — a settled keystroke, an
  // immediate facet toggle, or a back/forward re-seed. The first run is a no-op: the
  // SSR `initial` already seeded page one.
  $effect(() => {
    void filters.applied; // track the debounced snapshot
    void searchUserID;
    untrack(() => {
      const readerChanged = searchUserID !== lastSearchUserID;
      lastSearchUserID = searchUserID;
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
          q: displayQuery(filters.applied.q.trim()),
          facets: activeFilterCount(filters.applied),
        });
      }
      // Same query: this is a re-seed (initial navigation, back/forward), not a new
      // search — reload the page being read instead of snapping back to the first.
      const sameQuery = searchKey === lastSearchKey && !readerChanged;
      lastSearchKey = searchKey;
      if (!sameQuery) activePage = 1;
      if (readerChanged) {
        // A new reader has a different result set; an old offset can be past its end.
        const url = new URL(location.href);
        url.searchParams.delete('page');
        // eslint-disable-next-line svelte/no-navigation-without-resolve -- in-place query update to the current URL
        replaceState(url, page.state);
      }
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
      // `initial` describes `page.url`. When that is not the address bar (the stale
      // restore above) it is a page the visitor is not on; the reload effect fetches
      // the right one, and re-seeding here would put the stale page back over it.
      if (browser && page.url.search !== location.search) return;
      activePage = nextPage;
      const next = makePaginator();
      next.seed(slice, pageOffset(nextPage));
      jobs = next;
    });
  });

  // Re-seed the filters on every real navigation (see restoreFiltersOnNavigate).
  if (untrack(() => standalone)) restoreFiltersOnNavigate(filters, geo);
  else syncOnNavigation(filters);
</script>

<!-- Opens the current search on a world map, over the list rather than instead of it. -->
{#snippet mapButton()}
  <JobsMapButtons
    blockedBy={filters.value.formerEmployers ? 'formerEmployers' : filters.value.watchlist ? 'watchlist' : null}
    onMap={() => {
      mapLoaded = true;
      mapOpen = true;
    }}
    onSwipe={standalone ? openSwipe : undefined}
  />
{/snippet}

{#snippet listControls()}
  <JobListControls {filters} {selectedSort} {openWithinAvailable} />
{/snippet}

<div class="flex gap-6">
  <JobsSidebar
    top={sidebarTop}
    {filters}
    exclude={excludeFacets}
    onOpen={() => (modalOpen = true)}
    canSave={standalone}
    matchAvailable={matchFilterAvailable}
  />

  <div class="min-w-0 flex-1">
    <ListToolbar
      total={displayItems.length > 0 ? listTotal : null}
      unit={listTotal === 1 ? 'job' : 'jobs'}
      atLeast={serverMatch && jobs.totalIsLowerBound}
      settled={jobs.status === 'ready'}
      announceKey={resultSetKey}
      onSwipe={standalone ? openSwipe : undefined}
      controls={listControls}
      leading={standalone ? mapButton : undefined}
    />

    <JobsFeedNotices
      {nudge}
      {matchSortHint}
      {minMatch}
      onSortByMatch={() => filters.setSort('match')}
      {matchNeedsSkills}
    />

    <JobSearchError error={jobs.status === 'error' ? null : feedCounts.error} onlyRateLimit onRetry={retrySearch} />

    {#if jobs.status === 'loading' && !holdingOver}
      <States state="loading">
        {#snippet row()}
          <JobRowSkeleton />
        {/snippet}
      </States>
    {:else if jobs.status === 'error'}
      <JobSearchError error={jobs.error} onRetry={retrySearch} />
    {:else if displayItems.length === 0}
      <JobsEmptyState
        kind={formerEmployersApplied ? 'formerEmployers' : watchlistApplied ? 'watchlist' : 'none'}
        onRelax={standalone && relaxTarget ? relaxFeed : undefined}
      />
    {:else}
      <JobsFeedList
        jobs={visibleJobs}
        {clientMatches}
        {minMatch}
        {serverMatch}
        {holdingOver}
        onHide={hidden.onHide}
        hideCompany={!!scope.company_slug}
      />

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

{#if standalone && mapLoaded && !filters.value.formerEmployers && !filters.value.watchlist}
  {#await import('./map/JobsMapDialog.svelte') then { default: JobsMapDialog }}
    <JobsMapDialog bind:open={mapOpen} store={filters} />
  {/await}
{/if}

<!-- Undo affordance for the hide gesture. Keyed by slug so hiding a second job while
     a toast is up restarts the countdown for the newer one. A hide-then-forget is
     still recoverable in Activity → Hidden. -->
{#if hidden.pending}
  {#key hidden.pending.slug}
    <HiddenToast onUndo={hidden.undo} onClose={hidden.close} />
  {/key}
{/if}

<FilterModal
  store={filters}
  {counts}
  exclude={excludeFacets}
  savedSearches={standalone}
  formerEmployersAvailable={standalone}
  watchlistAvailable={standalone}
  hideSeenAvailable={standalone}
  open={modalOpen}
  onClose={() => (modalOpen = false)}
  {stagedCounts}
  matchAvailable={matchFilterAvailable}
/>
