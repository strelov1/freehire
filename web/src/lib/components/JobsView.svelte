<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { api, type Slice } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { ensureViewedLoaded } from '$lib/viewedJobs.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import { FilterStore, filtersToParams } from '$lib/filters.svelte';
  import type { Job, FacetCounts } from '$lib/types';
  import { Input } from '$lib/ui';
  import FiltersPanel from './FiltersPanel.svelte';
  import States from './States.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';

  // Filters live in the URL; the route `load` searches by them and returns the
  // first page as `initial`, so the rows are in the initial HTML. Each filter
  // change is a real navigation (FilterStore commits via goto), which re-runs
  // `load` and delivers a fresh `initial` here; "load more" then pages the rest
  // client-side over the same filters.
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

  // The user's facet filters plus the fixed `scope` params (company_slug, …).
  const scopedParams = () => {
    const p = filtersToParams(filters.value);
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
  let primed = false;

  // For a signed-in user, load the set of already-viewed slugs so JobRow can dim
  // seen cards (no-op when signed out — the set stays empty). Cleanup: cancel any
  // debounced filter navigation so it can't fire after this view is gone.
  onMount(() => {
    if (isAuthenticated()) ensureViewedLoaded();
    return () => filters.dispose();
  });

  // Keep the panel synced with the URL and refresh its facet counts. Runs on
  // mount and on every URL change — a filter commit (goto) or browser
  // back/forward. Track only the URL: syncFromUrl reads filters.value internally,
  // so untrack stops this from looping on its own write. It's a no-op when
  // already in sync (our own commits) and re-seeds the panel on back/forward;
  // either way the counts refresh for the new filters.
  $effect(() => {
    page.url.search; // track
    untrack(() => {
      filters.syncFromUrl();
      refreshCounts();
    });
  });

  // The list mirrors the route `load`: each filter change navigates and re-runs
  // it, so a fresh `initial` arrives as this prop — reseed a paginator from it.
  // The first run is the server-rendered page already seeded above, so skip it.
  $effect(() => {
    const slice = initial; // track the prop
    if (!primed) {
      primed = true;
      return;
    }
    const next = makePaginator();
    next.seed(slice);
    jobs = next;
  });
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 max-h-[calc(100vh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <FiltersPanel store={filters} exclude={excludeFacets} {counts} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <div class="mb-4 flex items-center gap-2">
      <Input
        type="search"
        value={filters.value.q}
        oninput={(e) => filters.setQuery(e.currentTarget.value)}
        placeholder="Search jobs…"
        aria-label="Search jobs"
        class="min-w-0 flex-1"
      />
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
        <LoadMore loading={jobs.loadingMore} error={jobs.loadMoreError} onclick={() => jobs.loadMore()} />
      {/if}
    {/if}
  </div>
</div>

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
