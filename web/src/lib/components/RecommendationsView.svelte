<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { resolve } from '$app/paths';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { isAuthenticated } from '$lib/auth.svelte';
  import { Paginator } from '$lib/paginated.svelte';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import type { Job, FacetCounts } from '$lib/types';
  import FilterSummary from './filters/FilterSummary.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import FilterEdgeTab from './FilterEdgeTab.svelte';
  import JobRow from './JobRow.svelte';
  import LoadMore from './LoadMore.svelte';
  import States from './States.svelte';

  // The recommendations feed is the open catalogue ranked by CV similarity, narrowed
  // by an optional facet filter. Filtering happens server-side — the vector search
  // ranks the whole catalogue while the page shows a slice, so a filter change
  // re-fetches the feed rather than filtering the current page. Filters live in the
  // URL but are NOT persisted to the shared /jobs storage key (persistence off), so
  // they stay per-page and never bleed into the standalone jobs list.
  const filters = new FilterStore(page.url.searchParams, false);

  const filterParams = () => filtersToParams(filters.applied);

  const makePaginator = () =>
    new Paginator<Job>((limit, offset) => api.recommendations(filterParams(), limit, offset));

  let feed = $state.raw(makePaginator());

  // Live facet counts feed the modal's controls and its "Show N jobs" total. For a
  // pure-vector feed the ranking spans the whole catalogue, so the facet distribution
  // over the same filters (the shared /jobs facets endpoint) is the honest proxy — it
  // answers "which facet values have open jobs", not "how many of your matches are X".
  let counts = $state<FacetCounts | null>(null);
  let countsGen = 0;
  const refreshCounts = () => {
    const gen = ++countsGen;
    return api
      .facetCounts(filterParams())
      .then((c) => {
        if (gen === countsGen) counts = c;
      })
      .catch(() => {});
  };

  // Disjunctive counts for the staged (in-modal) filter set, so a selected facet still
  // shows its siblings' counts and the total matches what the list would show.
  const stagedCounts = (params: URLSearchParams) => api.facetCounts(params, { disjunctive: true });

  let modalOpen = $state(false);

  function reloadFeed() {
    const next = makePaginator();
    next.start();
    feed = next;
  }

  // Load, and reload on every debounced filter change, but only when signed in — the
  // endpoint is authenticated. Unlike /jobs there is no SSR-seeded first page, so the
  // first run fetches too. `filters.applied` and `isAuthenticated()` are the tracked
  // deps; the reload itself is untracked so it reads the params without subscribing.
  $effect(() => {
    void filters.applied;
    if (!isAuthenticated()) return;
    untrack(() => {
      refreshCounts();
      reloadFeed();
    });
  });

  // Cancel the store's pending debounce when the view unmounts.
  onMount(() => () => filters.dispose());

  // Re-seed filters from the URL on every real navigation (back/forward, nav link).
  syncOnNavigation(filters);

  // An empty feed is ambiguous at the API — no-CV and no-match both return [] — so
  // disambiguate by whether a filter is active: a filtered-empty result is by
  // construction a filter outcome, and the no-CV case is independent of filters.
  // Keyed on the *applied* (debounced) params the feed was fetched with — not the
  // live filter count — so a mid-drag slider can't flash "no matches" before the
  // feed re-settles.
  const filtered = $derived([...filterParams()].length > 0);
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 flex max-h-[calc(100vh-5rem)] flex-col gap-4 overflow-y-auto">
      <div class="rounded-xl border border-border bg-card p-4">
        <FilterSummary store={filters} onOpen={() => (modalOpen = true)} />
      </div>
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    {#if feed.status === 'loading'}
      <States state="loading" />
    {:else if feed.status === 'error'}
      <States state="error" message="Couldn't load your recommendations." />
    {:else if feed.items.length === 0}
      {#if filtered}
        <States state="empty" message="No recommendations match these filters." />
      {:else}
        <div class="rounded-xl border border-border bg-card p-6 text-center">
          <p class="text-sm font-medium text-foreground">No recommendations yet</p>
          <p class="mt-1 text-sm text-muted-foreground">
            Add or update your CV on your
            <a
              href={resolve('/my/profile')}
              class="font-medium text-foreground underline underline-offset-4 hover:no-underline">profile</a
            >
            to get jobs matched to your experience.
          </p>
        </div>
      {/if}
    {:else}
      <ul class="flex flex-col gap-3">
        {#each feed.items as job (job.public_slug)}
          <li><JobRow {job} /></li>
        {/each}
      </ul>
      {#if feed.hasMore}
        <LoadMore loading={feed.loadingMore} error={feed.loadMoreError} onclick={() => feed.loadMore()} />
      {/if}
    {/if}
  </div>
</div>

<!-- Mobile left-edge Filters tab (desktop uses the sidebar summary above). -->
<FilterEdgeTab active={filters.active} onclick={() => (modalOpen = true)} />

<FilterModal
  store={filters}
  {counts}
  savedSearches={false}
  open={modalOpen}
  onClose={() => (modalOpen = false)}
  {stagedCounts}
/>
