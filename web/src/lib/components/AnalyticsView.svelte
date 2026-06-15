<script lang="ts">
  import { untrack } from 'svelte';
  import { page } from '$app/state';
  import { api } from '$lib/api';
  import { FilterStore, filtersToParams } from '$lib/filters.svelte';
  import { FACETS } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import FiltersPanel from './FiltersPanel.svelte';
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
  let drawerOpen = $state(false);
  let started = false;
  let timer: ReturnType<typeof setTimeout>;
  // Monotonic fetch id: a response only commits if it is still the latest, so a
  // slow fetch that resolves after a newer one can't overwrite fresh counts with
  // stale data (the debounce alone doesn't cover already-started fetches).
  let generation = 0;

  // Browser back/forward changes the URL query — pull it back into the filters.
  // Track only the URL (untrack the sync, which reads filters.value) so this does
  // not also fire on our own #commit writes. Same pattern as JobsView.
  $effect(() => {
    page.url.search; // track
    untrack(() => filters.syncFromUrl());
  });

  // Re-fetch counts when any filter changes, debounced. The first run is the
  // seeded SSR snapshot, so skip it. The effect's cleanup clears a pending timer
  // before the next run and on unmount (the debounce).
  $effect(() => {
    filtersToParams(filters.value).toString(); // track every filter field
    if (!started) {
      started = true;
      return;
    }
    status = 'loading';
    const params = filtersToParams(filters.value);
    const gen = ++generation;
    timer = setTimeout(async () => {
      try {
        const next = await api.facetCounts(params);
        if (gen !== generation) return; // a newer fetch superseded this one
        counts = next;
        status = 'ready';
      } catch {
        if (gen === generation) status = 'error';
      }
    }, 300);
    return () => clearTimeout(timer);
  });

  // The salary range across the matched set: the floor of salary_min and the
  // ceiling of salary_max (numeric facets are exposed as stats, not bars).
  const salary = $derived.by(() => {
    const lo = counts.stats.salary_min?.min;
    const hi = counts.stats.salary_max?.max;
    return lo != null && hi != null && hi > 0 ? { lo, hi } : null;
  });
</script>

<div class="flex gap-6">
  <aside class="hidden w-72 shrink-0 md:block">
    <div class="sticky top-6 max-h-[calc(100vh-5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <FiltersPanel store={filters} />
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
            Salary range: {salary.lo.toLocaleString()}–{salary.hi.toLocaleString()}
          </p>
        {/if}
      </div>
      <button
        type="button"
        class="h-9 shrink-0 rounded-lg border border-border bg-secondary px-3 text-sm font-medium text-secondary-foreground transition-colors hover:bg-accent md:hidden"
        onclick={() => (drawerOpen = true)}
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

{#if drawerOpen}
  <div class="fixed inset-0 z-40 md:hidden">
    <button class="absolute inset-0 bg-black/40" aria-label="Close filters" onclick={() => (drawerOpen = false)}></button>
    <div class="absolute left-0 top-0 flex h-full w-80 max-w-[85%] flex-col overflow-y-auto bg-background p-4 shadow-xl">
      <div class="mb-3 flex items-center justify-between">
        <span class="text-sm font-semibold tracking-tight">Filters</span>
        <button type="button" class="text-sm text-muted-foreground hover:text-foreground" onclick={() => (drawerOpen = false)}>Done</button>
      </div>
      <FiltersPanel store={filters} />
    </div>
  </div>
{/if}
