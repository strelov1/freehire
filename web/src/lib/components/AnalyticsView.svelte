<script lang="ts">
  import { onMount, untrack } from 'svelte';
  import { browser } from '$app/env';
  import { page } from '$app/state';
  import { api } from '#lib/api.js';
  import { FilterStore, filtersToParams } from '#lib/filters.js';
  import { syncOnNavigation } from '#lib/urlSynced.svelte.js';
  import { FACETS, countryLabel } from '#lib/facets.js';
  import type { FacetCounts } from '#lib/types.js';
  import type { InsightSalaryBand } from '#lib/api.js';
  import { formatSalary, salaryScopeFromParams, scopeBands, seniorityLabel } from '#lib/insights.js';
  import { categoryLabel } from '#lib/labels.js';
  import FilterSummary from './filters/FilterSummary.svelte';
  import FilterModal from './filters/FilterModal.svelte';
  import FilterEdgeTab from './FilterEdgeTab.svelte';
  import FacetBreakdown from './FacetBreakdown.svelte';
  import States from './States.svelte';

  // The first counts are server-rendered (route `load`) and arrive as `initial`,
  // so the breakdowns are in the initial HTML. Filters live in the URL; changing
  // one re-fetches the counts client-side over the same filter model as /jobs.
  let { initial, initialSalary }: { initial: FacetCounts; initialSalary: InsightSalaryBand[] } = $props();

  // Seed filters from the URL so server and hydrated client render the same view.
  const filters = new FilterStore(new URLSearchParams(page.url.search));

  // Reassigned wholesale on each fetch (an API response), so $state.raw avoids
  // proxying a large object we never mutate in place. Seeded once from the SSR
  // prop (untrack marks it an intentional snapshot, not a live binding).
  let counts = $state.raw<FacetCounts>(untrack(() => initial));
  let salaryBands = $state.raw<InsightSalaryBand[]>(untrack(() => initialSalary));
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
      const sal = salaryScopeFromParams(params);
      // A salary read failing leaves the counts standing: no figure, not an error page.
      Promise.all([api.facetCounts(params), sal ? api.insightsSalary(sal.scope).catch(() => []) : []])
        .then(([next, bands]) => {
          if (gen !== generation) return; // a newer fetch superseded this one
          counts = next;
          salaryBands = bands;
          status = 'ready';
        })
        .catch(() => {
          if (gen === generation) status = 'error';
        });
    });
  });

  // Median pay comes from the role × grade × country rollup, so only those three
  // filters shape it; the note says so when other filters narrow the counts.
  const salaryScope = $derived(salaryScopeFromParams(filtersToParams(filters.applied)));
  const medianBands = $derived(salaryScope ? scopeBands(salaryBands, salaryScope.scope) : []);
  const scopeName = $derived.by(() => {
    const s = salaryScope?.scope;
    if (!s) return '';
    const parts = [s.category ? categoryLabel(s.category) : 'All roles'];
    if (s.seniority) parts.push(seniorityLabel(s.seniority));
    if (s.country) parts.push(countryLabel(s.country));
    return parts.join(' · ');
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
    <!-- `top-20` is the site header's own `h-14` plus 24px of air. At `top-6` the card
         pinned 24px from the VIEWPORT, so the opaque header (`sticky top-0`) covered its
         top border and first row of controls for the whole scroll. The max-height is
         then what is left of the viewport, less the same 24px at the bottom. -->
    <div class="sticky top-20 max-h-[calc(100vh-6.5rem)] overflow-y-auto rounded-xl border border-border bg-card p-4">
      <FilterSummary store={filters} onOpen={() => (modalOpen = true)} />
    </div>
  </aside>

  <div class="min-w-0 flex-1">
    <div class="mb-4">
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

    <section class="mb-4 rounded-xl border border-border bg-card p-4" aria-labelledby="median-pay">
      <h2 id="median-pay" class="text-sm font-semibold">Median advertised salary</h2>
      {#if !salaryScope}
        <p class="mt-1 text-sm text-muted-foreground">
          Pick at most one specialization, one seniority and one country, with no exclusions, to see pay.
        </p>
      {:else if medianBands.length === 0}
        <p class="mt-1 text-sm text-muted-foreground">
          {scopeName}: too few open postings state a salary to show one.
        </p>
      {:else}
        <p class="mt-0.5 text-xs text-muted-foreground">{scopeName}, open postings that state pay</p>
        <ul class="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-3">
          {#each medianBands as b (`${b.currency}-${b.period}`)}
            <li>
              <p class="text-xl font-semibold tracking-tight tabular-nums">
                {formatSalary(b.p50, b.currency)}<span class="text-sm font-normal text-muted-foreground">
                  / {b.period}</span
                >
              </p>
              <p class="text-xs text-muted-foreground tabular-nums">
                middle half {formatSalary(b.p25, b.currency)}–{formatSalary(b.p75, b.currency)} · {b.sample_size.toLocaleString()}
                postings
              </p>
            </li>
          {/each}
        </ul>
        {#if salaryScope.partial}
          <p class="mt-3 text-xs text-muted-foreground">
            Only specialization, seniority and country shape this figure; your other filters do not.
          </p>
        {/if}
      {/if}
    </section>

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
<FilterEdgeTab active={filters.active} side="right" onclick={() => (modalOpen = true)} />

<FilterModal store={filters} {counts} open={modalOpen} onClose={() => (modalOpen = false)} {previewCount} />
