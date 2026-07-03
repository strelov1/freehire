<script lang="ts">
  import { ChevronDown, ChevronRight, Search } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import { countryLabel, type FacetStore } from '$lib/facets';
  import { REGION_LABELS } from '$lib/labels';
  import { COUNTRY_REGION_MAP, CITY_COUNTRY_MAP } from '$lib/generated/contracts';
  import type { FacetCounts } from '$lib/types';
  import { pillClass } from '../facets/pill';

  // Location pane: a region → country → city chip tree. The hierarchy is built from
  // the exported country→region / city→country maps, scoped to what actually has
  // jobs via the live facet distribution (so we don't render 200 empty countries).
  // Selection stages the existing regions/countries/cities params independently.
  let { store, counts = null }: { store: FacetStore; counts?: FacetCounts | null } = $props();

  let query = $state('');
  const expandedRegions = new SvelteSet<string>();
  const expandedCountries = new SvelteSet<string>();

  const regionCount = (code: string) => counts?.facets?.regions?.[code] ?? 0;
  const countryCounts = $derived(counts?.facets?.countries ?? {});
  const cityCounts = $derived(counts?.facets?.cities ?? {});

  // country code → cities (present in the distribution) that map to it, busiest first
  const citiesByCountry = $derived.by(() => {
    const out: Record<string, string[]> = {};
    for (const city of Object.keys(cityCounts)) {
      const cc = (CITY_COUNTRY_MAP as Record<string, string>)[city];
      if (!cc) continue;
      (out[cc] ??= []).push(city);
    }
    for (const arr of Object.values(out)) arr.sort((a, b) => (cityCounts[b] ?? 0) - (cityCounts[a] ?? 0));
    return out;
  });

  // region code → countries (present in the distribution) that map to it, busiest first
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

  // Regions to show: the curated macro-regions that have jobs or mapped countries.
  const regions = $derived(
    Object.keys(REGION_LABELS).filter((code) => regionCount(code) > 0 || (countriesByRegion[code]?.length ?? 0) > 0),
  );

  // Cities present in the distribution with no country mapping. The cities facet is a
  // huge open vocabulary, so we never dump the whole tail: by default only the top few
  // (busiest) are surfaced; the rest are reachable by typing in the search box.
  const ORPHAN_LIMIT = 12;
  const orphanCities = $derived(
    Object.keys(cityCounts)
      .filter((c) => !(CITY_COUNTRY_MAP as Record<string, string>)[c])
      .sort((a, b) => (cityCounts[b] ?? 0) - (cityCounts[a] ?? 0)),
  );

  const q = $derived(query.trim().toLowerCase());
  const matchCountry = (code: string) => !q || countryLabel(code).toLowerCase().includes(q);
  const matchCity = (city: string) => !q || city.toLowerCase().includes(q);

  const regionSel = $derived(store.facet('regions').values);
  const countrySel = $derived(store.facet('countries').values);
  const citySel = $derived(store.facet('cities').values);
  const fmt = (n: number) => n.toLocaleString('en-US');
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
    <button type="button" class="flex w-full items-center gap-2 py-3" onclick={() => (expandedRegions.has(region) ? expandedRegions.delete(region) : expandedRegions.add(region))}>
      <ChevronDown class={['size-4 text-muted-foreground transition-transform', !isOpen && '-rotate-90']} />
      <h3 class="text-sm font-semibold tracking-tight">{REGION_LABELS[region] ?? region}</h3>
      {#if regionCount(region)}<span class="text-xs text-muted-foreground">{fmt(regionCount(region))}</span>{/if}
    </button>

    {#if isOpen}
      <div class="flex flex-wrap gap-2 pb-2">
        <button
          type="button"
          onclick={() => store.toggle('regions', region)}
          class={pillClass(regionSel.includes(region), false, 'px-3 py-1.5 text-sm')}
        >
          All {REGION_LABELS[region] ?? region}
        </button>
        {#each countryCodes as code (code)}
          {@const cities = (citiesByCountry[code] ?? []).filter(matchCity)}
          <span class="inline-flex items-center">
            <button
              type="button"
              onclick={() => store.toggle('countries', code)}
              class={pillClass(countrySel.includes(code), false, 'px-3 py-1.5 text-sm')}
            >
              {countryLabel(code)}
            </button>
            {#if cities.length}
              <button
                type="button"
                aria-label="Show cities in {countryLabel(code)}"
                onclick={() => (expandedCountries.has(code) ? expandedCountries.delete(code) : expandedCountries.add(code))}
                class="ml-1 flex size-6 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
              >
                <ChevronRight class={['size-3.5 transition-transform', (expandedCountries.has(code) || q) && 'rotate-90']} />
              </button>
            {/if}
          </span>
        {/each}
      </div>

      {#each countryCodes as code (code)}
        {@const cities = (citiesByCountry[code] ?? []).filter(matchCity)}
        {#if (expandedCountries.has(code) || q) && cities.length}
          <div class="mb-2 ml-2 border-l-2 border-border pl-3">
            <div class="mb-1.5 text-[11px] text-muted-foreground">{countryLabel(code)} · cities</div>
            <div class="flex flex-wrap gap-2">
              {#each cities as city (city)}
                <button type="button" onclick={() => store.toggle('cities', city)} class={pillClass(citySel.includes(city), false, 'px-3 py-1.5 text-sm')}>
                  {city}
                </button>
              {/each}
            </div>
          </div>
        {/if}
      {/each}
    {/if}
  </div>
{/each}

{#if orphanCities.length}
  {@const matched = orphanCities.filter(matchCity)}
  {@const shown = q ? matched : matched.slice(0, ORPHAN_LIMIT)}
  {#if shown.length}
    <div class="border-t border-border pt-3">
      <div class="mb-2 text-sm font-semibold tracking-tight">Other cities</div>
      <div class="flex flex-wrap gap-2">
        {#each shown as city (city)}
          <button type="button" onclick={() => store.toggle('cities', city)} class={pillClass(citySel.includes(city), false, 'px-3 py-1.5 text-sm')}>
            {city}
          </button>
        {/each}
      </div>
      {#if !q && matched.length > shown.length}
        <p class="mt-2 text-xs text-muted-foreground">Type above to search {matched.length.toLocaleString('en-US')} more cities.</p>
      {/if}
    </div>
  {/if}
{/if}
