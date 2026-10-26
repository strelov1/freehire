<script lang="ts">
  import { SearchX } from '@lucide/svelte';
  import { highlightParts } from '#lib/fuzzy.js';
  import { matchFilterSearch, type FilterSearchHit } from '#lib/filterSearch.js';
  import { reportedCount } from '#lib/facets.js';
  import type { FacetCounts } from '#lib/types.js';
  import { pillClass, pillTitle } from '../facets/pill';

  let {
    items,
    query,
    counts = null,
    onToggle,
    onJump,
    selection,
  }: {
    items: readonly FilterSearchHit[];
    query: string;
    counts?: FacetCounts | null;
    onToggle: (hit: FilterSearchHit) => void;
    onJump: (railKey: string) => void;
    selection: (hit: FilterSearchHit) => { included: boolean; excluded: boolean; excludable: boolean };
  } = $props();

  const hits = $derived(matchFilterSearch(items, query));
  const groups = $derived.by(() => {
    const order: { key: string; group: string; railKey: string; railLabel: string; hits: FilterSearchHit[] }[] = [];
    const byKey: Record<string, (typeof order)[number]> = {};
    for (const hit of hits) {
      const key = `${hit.railKey}\0${hit.group}`;
      let bucket = byKey[key];
      if (!bucket) {
        bucket = { key, group: hit.group, railKey: hit.railKey, railLabel: hit.railLabel, hits: [] };
        byKey[key] = bucket;
        order.push(bucket);
      }
      bucket.hits.push(hit);
    }
    return order;
  });

  function activate(hit: FilterSearchHit) {
    if ((hit.param && hit.value != null) || hit.action) onToggle(hit);
    else onJump(hit.railKey);
  }

  function countOf(hit: FilterSearchHit): number | undefined {
    if (!hit.param || hit.value == null) return undefined;
    const dist = counts?.facets?.[hit.param];
    if (!dist) return undefined;
    return reportedCount(dist[hit.value] ?? 0);
  }
</script>

{#if groups.length === 0}
  <div class="flex h-full flex-col items-center justify-center gap-3 px-6 text-center">
    <div class="flex size-12 items-center justify-center rounded-full bg-brand/10 text-brand">
      <SearchX class="size-5" />
    </div>
    <p class="text-sm font-medium text-foreground">“{query.trim()}” isn't a filter</p>
    <p class="max-w-xs text-sm text-muted-foreground">Try a role, a skill, or a city. Typos are welcome.</p>
  </div>
{:else}
  <div class="flex flex-col gap-5">
    {#each groups as g (g.key)}
      <div>
        <div class="mb-2 flex items-baseline justify-between gap-3">
          <h3 class="text-sm font-semibold tracking-tight">{g.group}</h3>
          {#if g.group !== g.railLabel}
            <button
              type="button"
              class="shrink-0 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
              onclick={() => onJump(g.railKey)}
            >
              {g.railLabel}
            </button>
          {/if}
        </div>
        <div class="flex flex-wrap gap-2">
          {#each g.hits as hit (hit.id)}
            {@const state = selection(hit)}
            {@const count = countOf(hit)}
            <button
              type="button"
              onclick={() => activate(hit)}
              title={pillTitle(state.included, state.excluded, state.excludable)}
              class={pillClass(state.included || state.excluded, state.excluded, 'px-3 py-1.5 text-sm')}
            >
              {#each highlightParts(hit.label, query) as part, i (i)}
                {#if part.match}<mark class="rounded-sm bg-foreground/15 text-inherit">{part.text}</mark>{:else}{part.text}{/if}
              {/each}{#if count != null}<span class="ml-1 opacity-60 tabular-nums">{count.toLocaleString()}</span>{/if}
            </button>
          {/each}
        </div>
      </div>
    {/each}
  </div>
{/if}
