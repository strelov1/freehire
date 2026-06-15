<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { page } from '$app/state';
  import { api, type Slice } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { ensureViewedLoaded } from '$lib/viewedJobs.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import { FilterStore, filtersToParams, type SortField } from '$lib/filters.svelte';
  import type { Job } from '$lib/types';
  import { Input } from '$lib/ui';
  import FiltersPanel from './FiltersPanel.svelte';
  import States from './States.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';

  // The first page is server-rendered (route `load`) and arrives as `initial`,
  // so the rows are in the initial HTML. Filters live in the URL; the list is
  // driven by the search endpoint (an empty query browses everything).
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

  let drawerOpen = $state(false);
  let started = false;
  let timer: ReturnType<typeof setTimeout>;

  // For a signed-in user, load the set of already-viewed slugs so JobRow can dim
  // seen cards (no-op when signed out — the set stays empty). Cleanup: a debounce
  // timer left running after unmount would start a fetch for a gone component.
  onMount(() => {
    if (isAuthenticated()) ensureViewedLoaded();
    return () => clearTimeout(timer);
  });

  // House select styling, mirrored from ApiKeysView.
  const sortClass =
    'h-9 shrink-0 rounded-lg border border-input bg-transparent px-3 text-sm transition-colors focus-visible:border-ring focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50 dark:bg-input/30';

  // Browser back/forward changes the URL query — pull it back into the filters.
  // Track only the URL: syncFromUrl reads filters.value internally, so without
  // untrack this effect would also fire on our own setQuery/#commit writes and
  // clobber the just-typed value against a not-yet-updated page.url.
  $effect(() => {
    page.url.search; // track
    untrack(() => filters.syncFromUrl());
  });

  // Re-run the search when any filter changes, debounced. The first effect run
  // is the initial mount, already server-rendered/seeded, so skip it.
  $effect(() => {
    filtersToParams(filters.value).toString(); // track every filter field
    if (!started) {
      started = true;
      return;
    }
    clearTimeout(timer);
    timer = setTimeout(() => {
      jobs = makePaginator();
      jobs.start();
    }, 300);
  });
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 max-h-[calc(100vh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <FiltersPanel store={filters} exclude={excludeFacets} />
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
      <select
        value={filters.value.sort}
        onchange={(e) => filters.setSort(e.currentTarget.value as SortField)}
        aria-label="Sort jobs by"
        class={sortClass}
      >
        <option value="posted_at">Date posted</option>
        <option value="created_at">Recently added</option>
      </select>
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
      <FiltersPanel store={filters} exclude={excludeFacets} />
    </div>
  </div>
{/if}
