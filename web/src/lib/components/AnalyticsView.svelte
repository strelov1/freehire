<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { browser } from '$app/environment';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { FilterStore, filtersToParams } from '$lib/filters';
  import { syncOnNavigation } from '$lib/urlSynced.svelte';
  import { FACETS } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import FilterSummary from './filters/FilterSummary.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import FilterEdgeTab from './FilterEdgeTab.svelte';
  import FacetBreakdown from './FacetBreakdown.svelte';
  import States from './States.svelte';

  // The first counts are server-rendered (route `load`) and arrive as `initial`,
  // so the breakdowns are in the initial HTML. Filters live in the URL; changing
  // one re-fetches the counts client-side over the same filter model as /jobs.
  let { initial }: { initial: FacetCounts } = $props();

  // Seed filters from the URL so server and hydrated client render the same view.
  const filters = new FilterStore(page.url.searchParams);

  // Reassigned wholesale on each fetch (an API response), so $state.raw avoids
  // proxying a large object we never mutate in place. Seeded once from the SSR
  // prop (untrack marks it an intentional snapshot, not a live binding).
  let counts = $state.raw<FacetCounts>(untrack(() => initial));
  let status = $state<'ready' | 'loading' | 'error'>('ready');
  let modalOpen = $state(false);
  let started = false;

  // Job-count preview for the modal's staged filters — the same facet call, total only.
  const previewCount = (params: URLSearchParams) => api.facetCounts(params).then((c) => c.total);
  // `initial` counts were fetched for page.url; if a shallow-routing back/forward
  // left page.url lagging the address bar, they're stale — re-fetch on the first run.
  const initialStale = browser && page.url.search !== location.search;
  // Monotonic fetch id: a response only commits if it is still the latest, so a
  // slow fetch that resolves after a newer one can't overwrite fresh counts with
  // stale data. The reload debounce now lives in FilterStore (value -> applied),
  // so there is no second timer here.
  let generation = 0;

  // Cancel a pending debounced apply on unmount so it can't fire afterwards.
  onMount(() => () => filters.dispose());

  // Browser back/forward re-seeds the filters from the URL; the resulting
  // `applied` change drives the counts effect below.
  syncOnNavigation(filters);

  // Re-fetch counts when the debounced filters change. The debounce already
  // happened in the primitive, so fetch immediately — the `generation` guard (not
  // a timer) is what protects against an out-of-order response. Skip the first
  // run: the SSR `initial` counts are already shown.
  $effect(() => {
    void filters.applied; // track the debounced snapshot
    untrack(() => {
      if (!started) {
        started = true;
        if (!initialStale) return;
      }
      status = 'loading';
      const params = filtersToParams(filters.applied);
      const gen = ++generation;
      api
        .facetCounts(params)
        .then((next) => {
          if (gen !== generation) return; // a newer fetch superseded this one
          counts = next;
          status = 'ready';
        })
        .catch(() => {
          if (gen === generation) status = 'error';
        });
    });
  });

  // The salary range is only meaningful within a single currency — the stats
  // aggregate across every currency in the matched set, so a cross-currency
  // min–max would mix e.g. USD and CLP. Exactly one *included* currency pins the
  // matched set to it (the include filter ORs to just that currency), so show the
  // range then, labelled with that currency, regardless of any excluded currencies.
  const salaryCurrency = $derived.by(() => {
    const st = filters.facet('salary_currency');
    return st.include.length === 1 ? st.include[0] : null;
  });
  const salary = $derived.by(() => {
    if (!salaryCurrency) return null;
    const lo = counts.stats.salary_min?.min;
    const hi = counts.stats.salary_max?.max;
    return lo != null && hi != null && hi > 0 ? { lo, hi } : null;
  });
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 max-h-[calc(100vh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <FilterSummary store={filters} onOpen={() => (modalOpen = true)} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <div class="mb-4 flex items-center justify-between gap-2">
      <div>
        <p class="text-2xl font-semibold tracking-tight tabular-nums">
          {counts.total.toLocaleString()}
        </p>
        <p class="text-sm text-muted-foreground" aria-live="polite">
          {counts.total === 1 ? 'vacancy' : 'vacancies'} under current filters{#if status === 'loading'}
            <span class="ml-1 italic">· updating…</span>
          {/if}
        </p>
        {#if salary}
          <p class="mt-1 text-xs text-muted-foreground">
            Salary range ({salaryCurrency}): {salary.lo.toLocaleString()}–{salary.hi.toLocaleString()}
          </p>
        {/if}
      </div>
      <button
        type="button"
        class="h-9 shrink-0 rounded-lg border border-border bg-secondary px-3 text-sm font-medium text-secondary-foreground transition-colors hover:bg-accent md:hidden"
        onclick={() => (modalOpen = true)}
      >
        Filters{#if filters.active > 0}&nbsp;({filters.active}){/if}
      </button>
    </div>

    {#if status === 'error'}
      <States state="error" message="Failed to load analytics." />
    {:else}
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {#each FACETS as def (def.param)}
          <FacetBreakdown {def} dist={counts.facets[def.param]} store={filters} />
        {/each}
      </div>
    {/if}
  </div>
</div>

<!-- Mobile: edge tab opens the same full-screen modal as the header button. -->
<FilterEdgeTab active={filters.active} onclick={() => (modalOpen = true)} />

<FilterModal store={filters} {counts} open={modalOpen} onClose={() => (modalOpen = false)} {previewCount} />
