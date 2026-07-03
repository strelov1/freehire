<script lang="ts">
  import { ChevronDown, Search } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { countryLabel, type FacetStore } from '$lib/facets';
  import { REGION_LABELS } from '$lib/labels';
  import { COUNTRY_REGION_MAP } from '$lib/generated/contracts';
  import type { FacetCounts } from '$lib/types';
  import { pillClass } from '../facets/pill';

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

  // region code → countries present in the distribution, busiest first
  const countriesByRegion = $derived.by(() => {
    const out: Record<string, string[]> = {};
    for (const code of Object.keys(countryCounts)) {
      const r = (COUNTRY_REGION_MAP as Record<string, string>)[code];
      if (!r) continue;
      (out[r] ??= []).push(code);
    }
    for (const arr of Object.values(out)) arr.sort((a, b) => (countryCounts[b] ?? 0) - (countryCounts[a] ?? 0));
    return out;
  });

  const regions = $derived(
    Object.keys(REGION_LABELS).filter((code) => regionCount(code) > 0 || (countriesByRegion[code]?.length ?? 0) > 0),
  );

  // All cities in the distribution, busiest first.
  const allCities = $derived(Object.keys(cityCounts).sort((a, b) => (cityCounts[b] ?? 0) - (cityCounts[a] ?? 0)));
  const CITY_LIMIT = 18;

  const q = $derived(query.trim().toLowerCase());
  const matchCountry = (code: string) => !q || countryLabel(code).toLowerCase().includes(q);
  const matchCity = (city: string) => !q || city.toLowerCase().includes(q);

  const regionSel = $derived(store.facet('regions').values);
  const countrySel = $derived(store.facet('countries').values);
  const citySel = $derived(store.facet('cities').values);
  const fmt = (n: number) => n.toLocaleString('en-US');

  const cityMatches = $derived(allCities.filter(matchCity));
  const citiesShown = $derived(q ? cityMatches : cityMatches.slice(0, CITY_LIMIT));
</script>

<div class="mb-4 flex items-center gap-2 rounded-lg border border-input px-3">
  <Search class="size-4 shrink-0 text-muted-foreground" />
  <input
    bind:value={query}
    placeholder="Search country or city…"
    class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
  />
</div>

{#each regions as region (region)}
  {@const countryCodes = (countriesByRegion[region] ?? []).filter(matchCountry)}
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
      <div class="flex flex-wrap gap-2 pb-3">
        <button
          type="button"
          onclick={() => store.toggle('regions', region)}
          class={pillClass(regionSel.includes(region), false, 'px-3 py-1.5 text-sm')}
        >
          All {REGION_LABELS[region] ?? region}
        </button>
        {#each countryCodes as code (code)}
          <button
            type="button"
            onclick={() => store.toggle('countries', code)}
            class={pillClass(countrySel.includes(code), false, 'px-3 py-1.5 text-sm')}
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
        <button type="button" onclick={() => store.toggle('cities', city)} class={pillClass(citySel.includes(city), false, 'px-3 py-1.5 text-sm')}>
          {city}
        </button>
      {/each}
    </div>
    {#if !q && cityMatches.length > citiesShown.length}
      <p class="mt-2 text-xs text-muted-foreground">Type above to search {fmt(cityMatches.length)} cities.</p>
    {/if}
  </div>
{/if}
