<script lang="ts">
  import { onMount, untrack, type Snippet } from 'svelte';
  import { Layers } from '@lucide/svelte';
  import { browser } from '$app/environment';
  import { afterNavigate, goto } from '$app/navigation';
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
  import { canFetchMore, pageCount, pageOffset } from '$lib/pagination';
  import Pagination from './Pagination.svelte';
  import { FilterStore, filtersToParams, activeFilterCount } from '$lib/filters';
  import { loadJobFilters } from '$lib/filterStorage';
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
  import { LoadMore } from '$lib/ui';
  import InfiniteScroll from './InfiniteScroll.svelte';
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
  // `currentPage` is set by a route whose `load` honours `?page=N` — it both seeds
  // the paginator at the right offset (so scrolling on continues the result set
  // instead of replaying earlier pages) and turns on the <a href> page nav under
  // the feed. Routes that always serve page one leave it unset and render no nav:
  // links there would every one of them lead back to the same first page.
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
    currentPage?: number;
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

  // Seeded with the server-rendered first page (an intentional one-time snapshot
  // of the initial prop); "load more" and filter changes fetch client-side.
  // The page being read. Starts at the route's `?page=N` and survives a reload that
  // didn't change the query; a changed query resets it to 1, because the visitor is
  // now looking at a different result set (and FilterStore has already dropped
  // `page` from the URL — it only ever writes facet params).
  let activePage = $state(untrack(() => currentPage) ?? 1);

  const seeded = makePaginator();
  seeded.seed(untrack(() => initial), pageOffset(untrack(() => currentPage) ?? 1));
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
  // funnel event.
  let lastSearchKey = '';

  // Onboarding: the one-time nudge banner + wizard, standalone-only. The lifecycle
  // lives in localStorage (client-only); seed it at init on the client so a returning
  // (dismissed/completed) visitor never flashes a banner before mount. The banner is
  // the sole entry — once dismissed or completed it retires; there is no persistent
  // re-open control.
  let wizardOpen = $state(false);
  let onboardingState = $state<OnboardingLifecycle>(browser ? loadOnboardingState() : 'unseen');
  // The ephemeral post-onboarding Telegram-alert offer (set after the wizard, or on
  // mount to resume a pending alert after sign-in). Dismissible; not persisted.
  let alertBanner = $state<{ query: string; autostart: boolean } | null>(null);
  // Show only to an un-nudged visitor with no active facet filters AND no text query,
  // so a shared search/filter link is never interrupted (activeFilterCount ignores the
  // query, so check it explicitly). Gated on `browser`: never SSR the banner.
  const showBanner = $derived(
    browser &&
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
      filterScope: { store: filters, counts: () => counts, variant: 'jobs' },
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

  // The feed minus the signed-in user's hidden jobs, and (when the match slider is
  // active) minus jobs below the chosen match threshold. Dismissal is cross-referenced
  // client-side against the shared dismissed set (loaded on mount), mirroring the
  // viewed/saved sets — the server search is untouched. Re-derives when the page
  // changes, when a hide/undo mutates the dismissed set, or when the slider moves, so
  // a hidden or filtered-out card drops (and an undone one returns) instantly. A job
  // with no skills has no percent to test (see computeClientMatch) and stays, matching
  // the card's own `no-skills` state, which shows no match at all rather than a false 0%.
  // The search API only serves the first SEARCH_WINDOW rows, however many matches it
  // reports. Past that, asking for another page returns 400 and the feed would show
  // "Couldn't load more" — an error for what is just the end of what's reachable.
  const moreReachable = $derived(canFetchMore(pageOffset(activePage), jobs.items.length));

  const visibleJobs = $derived(
    jobs.items.filter((j) => {
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
      else filters.syncFromUrl();
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
      total={jobs.items.length > 0 ? listTotal : null}
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

    {#if jobs.status === 'loading'}
      <States state="loading" />
    {:else if jobs.status === 'error'}
      <States state="error" message="Failed to load jobs." />
    {:else if jobs.items.length === 0}
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
    {:else if visibleJobs.length === 0 && !jobs.hasMore}
      <!-- The server returned jobs but the user has hidden every one on this final
           page: show the empty state rather than a blank feed. (With more pages,
           the {:else} below keeps InfiniteScroll loading instead.) -->
      <States state="empty" message="No matching jobs." />
    {:else}
      <div class="flex flex-col gap-3">
        {#each visibleJobs as job (job.public_slug)}
          <JobRow {job} {onHide} />
        {/each}
      </div>

      {#if jobs.hasMore && moreReachable}
        <!-- Scroll-to-bottom auto-load; the button stays as the accessible
             fallback (keyboard/screen-reader, and retry on a failed load). -->
        <InfiniteScroll onLoad={() => jobs.loadMore()} enabled={!jobs.loadingMore && !jobs.loadMoreError} />
        <LoadMore loading={jobs.loadingMore} error={jobs.loadMoreError} onclick={() => jobs.loadMore()} />
      {/if}

      {#if currentPage !== undefined}
        <Pagination
          current={activePage}
          total={pageCount(jobs.total)}
          pathname={page.url.pathname}
          params={page.url.searchParams}
        />
      {/if}
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
