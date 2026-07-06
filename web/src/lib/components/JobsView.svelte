<script lang="ts">
  import { onMount, untrack, type Snippet } from 'svelte';
  import { Layers } from '@lucide/svelte';
  import { browser } from '$app/environment';
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api, type Slice } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { ensureViewedLoaded } from '$lib/viewedJobs.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import { loadJobFilters } from '$lib/filterStorage';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { setListSearchTarget } from '$lib/listSearch.svelte';
  import type { Job, FacetCounts } from '$lib/types';
  import FilterSummary from './filters/FilterSummary.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import FilterEdgeTab from './FilterEdgeTab.svelte';
  import States from './States.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import InfiniteScroll from './InfiniteScroll.svelte';

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
  let {
    initial,
    scope = {},
    excludeFacets = [],
    sidebarTop,
  }: {
    initial: Slice<Job>;
    scope?: Record<string, string>;
    excludeFacets?: string[];
    sidebarTop?: Snippet;
  } = $props();

  // Standalone /jobs (no fixed scope) hands its text search to the header; an
  // embedded, scoped instance (e.g. a company page) keeps its own inline input.
  const standalone = $derived(Object.keys(scope).length === 0);

  // Seed filters from the current URL so the server and the hydrated client
  // render the same filtered view. Only the standalone list persists to storage;
  // the embedded company list must not clobber the shared key. Persistence is
  // fixed for the store's life, so the initial `standalone` is captured once.
  const filters = new FilterStore(page.url.searchParams, untrack(() => standalone));

  // The user's (debounced) facet filters plus the fixed `scope` params
  // (company_slug, …). Reads `applied` so typing doesn't fetch per keystroke.
  const scopedParams = () => {
    const p = filtersToParams(filters.applied);
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return p;
  };

  const makePaginator = () =>
    new Paginator<Job>((limit, offset) => api.searchJobs(scopedParams(), limit, offset));

  // Seeded with the server-rendered first page (an intentional one-time snapshot
  // of the initial prop); "load more" and filter changes fetch client-side.
  const seeded = makePaginator();
  seeded.seed(untrack(() => initial));
  let jobs = $state.raw(seeded);

  // The live facet distribution (value → count per facet), feeding the dynamic
  // selects (skills, countries) so the user sees which values exist and how many
  // jobs each has under the current filters. A failed fetch leaves the prior
  // counts — the selects degrade to plain (countless) options, never break.
  // `countsGen` is a monotonic fetch id so a slow earlier response can't
  // overwrite a newer one (same guard as AnalyticsView).
  let counts = $state<FacetCounts | null>(null);
  let countsGen = 0;
  const refreshCounts = () => {
    const gen = ++countsGen;
    return api
      .facetCounts(scopedParams())
      .then((c) => {
        if (gen === countsGen) counts = c;
      })
      .catch(() => {});
  };

  let modalOpen = $state(false);
  let started = false;

  // The job count for a staged filter set (built by the modal), merged with the fixed
  // scope params (e.g. company_slug) so the "Show N jobs" preview matches the list.
  const previewCount = (params: URLSearchParams) => {
    const p = new URLSearchParams(params);
    for (const [k, v] of Object.entries(scope)) p.set(k, v);
    return api.facetCounts(p).then((c) => c.total);
  };

  // The server-rendered `initial` was searched for `page.url`. After a back/forward
  // onto a shallow-routing entry that lags page.url behind the address bar, `initial`
  // is stale (it holds the pre-filter page) while the filters seed from the real URL.
  // Detect that mismatch so the first effect run reloads instead of keeping `initial`.
  const initialStale = browser && page.url.search !== location.search;

  // For a signed-in user, load the set of already-viewed slugs so JobRow can dim
  // seen cards (no-op when signed out — the set stays empty). Cleanup: cancel any
  // pending debounced reload so it can't fire after this view is gone.
  onMount(() => {
    if (isAuthenticated()) ensureViewedLoaded();
    // Register this page's store so the header search drives it. This holds for the
    // standalone /jobs list AND the company page's embedded, scoped list — on
    // /companies/:slug the header search filters that company's postings (there's
    // no inline box), so both modes route their text search through the header.
    setListSearchTarget(filters);
    return () => {
      setListSearchTarget(null);
      filters.dispose();
    };
  });

  function reloadList() {
    const next = makePaginator();
    next.start();
    jobs = next;
  }

  // Enter swipe mode carrying the current filters + query (same param shape the
  // list uses), so the deck reflects exactly what's on screen. `scope` params are
  // fixed context (e.g. company_slug) and go along too.
  function openSwipe() {
    const params = scopedParams();
    const qs = params.toString();
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
      if (!started) {
        started = true;
        // Keep the SSR `initial` page unless it was loaded for a different URL than
        // the address bar (stale shallow-routing restore) — then reload to match.
        if (!initialStale) return;
      }
      reloadList();
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
            {jobs.total.toLocaleString()}
          </p>
          <p class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {jobs.total === 1 ? 'open job' : 'open jobs'}
          </p>
        </div>
      {/if}
      {@render sidebarTop?.()}
      <div class="rounded-xl border border-border bg-card p-4">
        <FilterSummary store={filters} exclude={excludeFacets} onOpen={() => (modalOpen = true)} />
      </div>
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    {#if jobs.status === 'loading'}
      <States state="loading" />
    {:else if jobs.status === 'error'}
      <States state="error" message="Failed to load jobs." />
    {:else if jobs.items.length === 0}
      <States state="empty" message="No matching jobs." />
    {:else}
      <!-- Standalone: the count is the topmost element, level with the fixed edge
           tab, so pl-12 clears it (reset at md). Company embed: the count sits well
           below the header/facts, clear of the tab, so it aligns with the cards; it
           also moves to the sidebar on desktop, hence mobile-only and a touch bolder. -->
      <p
        class={[
          'mb-3 text-sm',
          standalone ? 'pl-12 text-muted-foreground md:pl-0' : 'font-medium text-foreground md:hidden',
        ]}
        aria-live="polite"
      >
        {jobs.total.toLocaleString()} {jobs.total === 1 ? 'job' : 'jobs'}
      </p>
      <div class="flex flex-col gap-3">
        {#each jobs.items as job (job.public_slug)}
          <JobRow {job} />
        {/each}
      </div>

      {#if jobs.hasMore}
        <!-- Scroll-to-bottom auto-load; the button stays as the accessible
             fallback (keyboard/screen-reader, and retry on a failed load). -->
        <InfiniteScroll onLoad={() => jobs.loadMore()} enabled={!jobs.loadingMore && !jobs.loadMoreError} />
        <LoadMore loading={jobs.loadingMore} error={jobs.loadMoreError} onclick={() => jobs.loadMore()} />
      {/if}
    {/if}
  </div>
</div>

<!-- Mobile left-edge Filters tab, for both the standalone list and the embedded
     company view (parity with /jobs) — it floats over the left edge of the content
     the same way on either page. -->
<FilterEdgeTab active={filters.active} onclick={() => (modalOpen = true)} />

{#if standalone}
  <!-- Swipe-mode entry: an icon-only button pinned to the right viewport edge,
       level with the left filters tab (top-16). Fixed, so it only exists while the
       standalone jobs list is mounted (never on the embedded company view) and
       stays reachable while scrolling; kept below the z-40 mobile overlays. -->
  <button
    type="button"
    onclick={openSwipe}
    aria-label="Swipe mode"
    title="Swipe mode"
    class="fixed right-0 top-16 z-30 flex items-center rounded-l-lg border border-r-0 border-border bg-secondary py-2.5 pl-2 pr-1.5 text-secondary-foreground shadow-sm transition-colors hover:bg-accent"
  >
    <Layers class="size-4 shrink-0" />
  </button>
{/if}

<FilterModal
  store={filters}
  {counts}
  exclude={excludeFacets}
  savedSearches={standalone}
  open={modalOpen}
  onClose={() => (modalOpen = false)}
  {previewCount}
/>
