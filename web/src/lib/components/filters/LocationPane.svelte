<script lang="ts">
  import { ChevronDown, Search, X } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { countryLabel, REGION_UNSPECIFIED, type FacetStore } from '$lib/facets';
  import { REGION_LABELS } from '$lib/labels';
  import { COUNTRY_REGION_MAP } from '$lib/generated/contracts';
  import type { FacetCounts } from '$lib/types';
  import { pillClass, pillTitle } from '../facets/pill';

  // Location pane: a region → country tree plus a flat, searchable Cities list.
  // The country tree is built from the exported country→region map, scoped to what
  // actually has jobs via the live facet distribution. Cities are a large LLM-open
  // vocabulary with no reliable country link, so they aren't nested under countries —
  // they live in one clear Cities section (busiest first, the rest via search).
  // Selecting at any level stages the existing regions/countries/cities params.
  let { store, counts = null }: { store: FacetStore; counts?: FacetCounts | null } = $props();

  let query = $state('');
  const expandedRegions = new SvelteSet<string>();

  const regionCount = (code: string) => counts?.facets?.regions?.[code] ?? 0;
  const countryCounts = $derived(counts?.facets?.countries ?? {});
  const cityCounts = $derived(counts?.facets?.cities ?? {});

  // regions/countries/cities are excludable: each pill cycles off → include →
  // exclude → off. The `*Sel` unions drive the "kept visible / has selection" logic;
  // the per-facet include/exclude sets drive each pill's state and style.
  const regionF = $derived(store.facet('regions'));
  const countryF = $derived(store.facet('countries'));
  const cityF = $derived(store.facet('cities'));
  const regionSel = $derived([...regionF.include, ...regionF.exclude]);
  const countrySel = $derived([...countryF.include, ...countryF.exclude]);
  const citySel = $derived([...cityF.include, ...cityF.exclude]);

  // region code → countries to show, busiest first. Built from the live distribution
  // plus any currently-selected country, so a selected country stays visible (and
  // deselectable) even when the current filters give it a zero count.
  const countriesByRegion = $derived.by(() => {
    const out: Record<string, string[]> = {};
    const add = (code: string) => {
      const r = (COUNTRY_REGION_MAP as Record<string, string>)[code];
      if (!r) return;
      (out[r] ??= []).push(code);
    };
    for (const code of Object.keys(countryCounts)) add(code);
    for (const code of countrySel) if (!countryCounts[code]) add(code);
    for (const arr of Object.values(out)) arr.sort((a, b) => (countryCounts[b] ?? 0) - (countryCounts[a] ?? 0));
    return out;
  });

  // The macro-regions are always shown (a stable list, like the old region facet), so
  // the pane is never empty — even before the first facet-count fetch resolves. `global`
  // is pulled out: no country maps to it, so it has nothing to drill into and renders as
  // a flat pill (see flatRegions) rather than an empty accordion.
  const regions = Object.keys(REGION_LABELS).filter((r) => r !== 'global');

  // Country-less region pills shown flat above the tree: the "anywhere" macro-bucket
  // (global) and the "no resolved geography" sentinel (REGION_UNSPECIFIED → "Not
  // specified"). Neither drills into countries, so both stay as one-click pills.
  const flatRegions = ['global', REGION_UNSPECIFIED];

  // Cities to show: the distribution plus any selected city (kept visible at zero
  // count), busiest first.
  const allCities = $derived.by(() => {
    const set = new Set([...Object.keys(cityCounts), ...citySel]);
    return [...set].sort((a, b) => (cityCounts[b] ?? 0) - (cityCounts[a] ?? 0));
  });
  const CITY_LIMIT = 18;

  const q = $derived(query.trim().toLowerCase());
  const regionLabel = (code: string) => (code === REGION_UNSPECIFIED ? 'Not specified' : (REGION_LABELS[code] ?? code));
  const matchRegion = (code: string) => !q || regionLabel(code).toLowerCase().includes(q);
  const matchCountry = (code: string) => !q || countryLabel(code).toLowerCase().includes(q);
  const matchCity = (city: string) => !q || city.toLowerCase().includes(q);

  const fmt = (n: number) => n.toLocaleString('en-US');

  const cityMatches = $derived(allCities.filter(matchCity));
  const citiesShown = $derived(q ? cityMatches : cityMatches.slice(0, CITY_LIMIT));

  // Regions to render, each with its matching countries. While searching, a region is
  // dropped unless its own name matches or it still has a matching country (its "All
  // region" pill then hides — see nameMatch); with no query every region stays.
  const visibleRegions = $derived(
    regions
      .map((region) => ({ region, countryCodes: (countriesByRegion[region] ?? []).filter(matchCountry), nameMatch: matchRegion(region) }))
      .filter((r) => r.nameMatch || r.countryCodes.length),
  );

  // Flat pills hide during a search unless their own label matches the query.
  const visibleFlatRegions = $derived(flatRegions.filter(matchRegion));

  // Selected-location chips, shown above the tree: included first, then excluded,
  // across regions → countries → cities. Each chip removes its value outright (not a
  // cycle) so the X is a one-click clear. Same shape/style as the sidebar summary.
  type GeoChip = { key: string; label: string; exclude: boolean; remove: () => void };
  const selectedChips = $derived.by((): GeoChip[] => {
    const out: GeoChip[] = [];
    const push = (param: string, values: string[], label: (v: string) => string, exclude: boolean) => {
      for (const v of values) out.push({ key: `${param}:${v}`, label: label(v), exclude, remove: () => store.remove(param, v) });
    };
    const cityText = (city: string) => city;
    push('regions', regionF.include, regionLabel, false);
    push('countries', countryF.include, countryLabel, false);
    push('cities', cityF.include, cityText, false);
    push('regions', regionF.exclude, regionLabel, true);
    push('countries', countryF.exclude, countryLabel, true);
    push('cities', cityF.exclude, cityText, true);
    return out;
  });

  const hasGeo = $derived(regionSel.length + countrySel.length + citySel.length > 0);
  function clearGeo() {
    store.clearFacet('regions');
    store.clearFacet('countries');
    store.clearFacet('cities');
  }
</script>

<div class="mb-2 flex min-h-6 items-center justify-between gap-2">
  <h3 class="text-sm font-semibold tracking-tight">Location</h3>
  {#if hasGeo}
    <button
      type="button"
      onclick={clearGeo}
      title="Clear Location"
      aria-label="Clear Location"
      class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
    >
      <X class="size-3.5" />
    </button>
  {/if}
</div>

<div class="mb-4 flex items-center gap-2 rounded-lg border border-input px-3">
  <Search class="size-4 shrink-0 text-muted-foreground" />
  <input
    bind:value={query}
    placeholder="Search country or city…"
    class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
  />
</div>

{#if selectedChips.length}
  <div class="mb-4 flex flex-wrap gap-1.5">
    {#each selectedChips as chip (chip.key)}
      <span
        class={[
          'inline-flex items-center gap-1 rounded-full border px-2.5 py-1 text-xs font-medium',
          chip.exclude ? 'border-destructive/30 bg-destructive/15 text-destructive line-through' : 'border-border bg-secondary text-secondary-foreground',
        ]}
      >
        {chip.label}
        <button type="button" aria-label="Remove {chip.label}" onclick={chip.remove} class="text-muted-foreground transition-colors hover:text-foreground">
          <X class="size-3" />
        </button>
      </span>
    {/each}
  </div>
{/if}

{#if visibleFlatRegions.length}
  <div class="mb-1 flex flex-wrap gap-2 border-b border-border pb-3">
    {#each visibleFlatRegions as code (code)}
      {@const rExc = regionF.exclude.includes(code)}
      {@const rInc = regionF.include.includes(code)}
      <button
        type="button"
        onclick={() => store.cycle('regions', code)}
        title={pillTitle(rInc, rExc, true)}
        class={pillClass(rInc || rExc, rExc, 'px-3 py-1.5 text-sm')}
      >
        {regionLabel(code)}
      </button>
    {/each}
  </div>
{/if}

{#each visibleRegions as { region, countryCodes, nameMatch } (region)}
  {@const isOpen = expandedRegions.has(region) || !!q}
  <div class="border-t border-border first:border-t-0">
    <button
      type="button"
      class="flex w-full items-center gap-2 py-3"
      onclick={() => (expandedRegions.has(region) ? expandedRegions.delete(region) : expandedRegions.add(region))}
    >
      <ChevronDown class={['size-4 text-muted-foreground transition-transform', !isOpen && '-rotate-90']} />
      <h3 class="text-sm font-semibold tracking-tight">{REGION_LABELS[region] ?? region}</h3>
      {#if regionCount(region)}<span class="text-xs text-muted-foreground">{fmt(regionCount(region))}</span>{/if}
    </button>

    {#if isOpen}
      {@const rExc = regionF.exclude.includes(region)}
      {@const rInc = regionF.include.includes(region)}
      <div class="flex flex-wrap gap-2 pb-3">
        {#if nameMatch}
          <button
            type="button"
            onclick={() => store.cycle('regions', region)}
            title={pillTitle(rInc, rExc, true)}
            class={pillClass(rInc || rExc, rExc, 'px-3 py-1.5 text-sm')}
          >
            All {REGION_LABELS[region] ?? region}
          </button>
        {/if}
        {#each countryCodes as code (code)}
          {@const cExc = countryF.exclude.includes(code)}
          {@const cInc = countryF.include.includes(code)}
          <button
            type="button"
            onclick={() => store.cycle('countries', code)}
            title={pillTitle(cInc, cExc, true)}
            class={pillClass(cInc || cExc, cExc, 'px-3 py-1.5 text-sm')}
          >
            {countryLabel(code)}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/each}

{#if citiesShown.length}
  <div class="border-t border-border pt-3">
    <div class="mb-2 text-sm font-semibold tracking-tight">Cities</div>
    <div class="flex flex-wrap gap-2">
      {#each citiesShown as city (city)}
        {@const ctExc = cityF.exclude.includes(city)}
        {@const ctInc = cityF.include.includes(city)}
        <button
          type="button"
          onclick={() => store.cycle('cities', city)}
          title={pillTitle(ctInc, ctExc, true)}
          class={pillClass(ctInc || ctExc, ctExc, 'px-3 py-1.5 text-sm')}
        >
          {city}
        </button>
      {/each}
    </div>
    {#if !q && cityMatches.length > citiesShown.length}
      <p class="mt-2 text-xs text-muted-foreground">Type above to search {fmt(cityMatches.length)} cities.</p>
    {/if}
  </div>
{/if}
