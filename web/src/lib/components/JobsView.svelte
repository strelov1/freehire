<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { Layers } from '@lucide/svelte';
  import { browser } from '$app/environment';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api, type Slice } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { ensureViewedLoaded } from '$lib/viewedJobs.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { setListSearchTarget } from '$lib/listSearch.svelte';
  import type { Job, FacetCounts } from '$lib/types';
  import { Input } from '$lib/ui';
  import FiltersPanel from './FiltersPanel.svelte';
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
  let {
    initial,
    scope = {},
    excludeFacets = [],
  }: { initial: Slice<Job>; scope?: Record<string, string>; excludeFacets?: string[] } = $props();

  // Seed filters from the current URL so the server and the hydrated client
  // render the same filtered view.
  const filters = new FilterStore(page.url.searchParams);

  // Standalone /jobs (no fixed scope) hands its text search to the header; an
  // embedded, scoped instance (e.g. a company page) keeps its own inline input.
  const standalone = $derived(Object.keys(scope).length === 0);

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

  let drawerOpen = $state(false);
  let started = false;

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
    // Register this page's store so the header search drives it (standalone only).
    if (standalone) setListSearchTarget(filters);
    return () => {
      if (standalone) setListSearchTarget(null);
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

  // Browser back/forward re-seeds the filters from the URL; the resulting
  // `applied` change drives the reload effect above.
  syncOnNavigation(filters);
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 max-h-[calc(100vh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <FiltersPanel store={filters} exclude={excludeFacets} {counts} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <div class="mb-4 flex items-center gap-2">
      {#if standalone}
        <!-- The header search is this page's text filter (see HeaderListSearch);
             the spacer keeps the Filters button right-aligned. -->
        <div class="flex-1"></div>
      {:else}
        <Input
          type="search"
          value={filters.value.q}
          oninput={(e) => filters.setQuery(e.currentTarget.value)}
          placeholder="Search jobs…"
          aria-label="Search jobs"
          class="min-w-0 flex-1"
        />
      {/if}
      <button
        type="button"
        class="h-9 shrink-0 rounded-lg border border-border bg-secondary px-3 text-sm font-medium text-secondary-foreground transition-colors hover:bg-accent md:hidden"
        onclick={() => (drawerOpen = true)}
      >
        Filters{#if filters.active > 0}&nbsp;({filters.active}){/if}
      </button>
    </div>

    {#if jobs.status === 'loading'}
      <States state="loading" />
    {:else if jobs.status === 'error'}
      <States state="error" message="Failed to load jobs." />
    {:else if jobs.items.length === 0}
      <States state="empty" message="No matching jobs." />
    {:else}
      <p class="mb-3 text-sm text-muted-foreground" aria-live="polite">
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

{#if standalone}
  <!-- Swipe-mode entry: an icon-only button pinned to the right viewport edge,
       level with the top of the filters panel (header h-14 + page py-6 = top-20).
       Fixed, so it only exists while the standalone jobs list is mounted (never on
       the embedded company view) and stays reachable while scrolling; kept below
       the z-40 mobile overlays. -->
  <button
    type="button"
    onclick={openSwipe}
    aria-label="Swipe mode"
    title="Swipe mode"
    class="fixed right-0 top-20 z-30 flex items-center rounded-l-lg border border-r-0 border-border bg-secondary p-3 text-secondary-foreground shadow-sm transition-colors hover:bg-accent"
  >
    <Layers class="h-5 w-5 shrink-0" />
  </button>
{/if}

{#if drawerOpen}
  <div class="fixed inset-0 z-40 md:hidden">
    <button class="absolute inset-0 bg-black/40" aria-label="Close filters" onclick={() => (drawerOpen = false)}></button>
    <div class="absolute left-0 top-0 flex h-full w-80 max-w-[85%] flex-col overflow-y-auto bg-background p-4 shadow-xl">
      <div class="mb-3 flex items-center justify-between">
        <span class="text-sm font-semibold tracking-tight">Filters</span>
        <button type="button" class="text-sm text-muted-foreground hover:text-foreground" onclick={() => (drawerOpen = false)}>Done</button>
      </div>
      <FiltersPanel store={filters} exclude={excludeFacets} {counts} />
    </div>
  </div>
{/if}
