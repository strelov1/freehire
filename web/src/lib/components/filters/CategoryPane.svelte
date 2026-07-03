<script lang="ts">
  import { ChevronDown, Search } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import type { FacetStore } from '$lib/facets';
  import { CATEGORY_GROUPS } from '$lib/filterSections';
  import FacetHeader from './FacetHeader.svelte';
  import { pillClass, pillTitle } from '../facets/pill';

  // Specialization pane: category chips grouped into collapsible sections, with a
  // facet-local search filtering the visible options. In the job filter it toggles
  // the excludable `category` facet (each chip cycles off → include → exclude → off);
  // `plain` makes it an include-only choice (e.g. a profile category is not a filter
  // to exclude).
  let { store, plain = false }: { store: FacetStore; plain?: boolean } = $props();

  const excludable = $derived(!plain);
  let query = $state('');
  const collapsed = new SvelteSet<string>();
  const st = $derived(store.facet('category'));
  const onToggle = (v: string) => (excludable ? store.cycle('category', v) : store.pick('category', v));

  const groups = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return CATEGORY_GROUPS.map((g) => ({
      ...g,
      options: q ? g.options.filter((o) => o.label.toLowerCase().includes(q)) : g.options,
    })).filter((g) => g.options.length > 0);
  });
</script>

<FacetHeader {store} param="category" label="Specialization" />

<div class="mb-4 flex items-center gap-2 rounded-lg border border-input px-3">
  <Search class="size-4 shrink-0 text-muted-foreground" />
  <input
    bind:value={query}
    placeholder="Search specializations…"
    class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
  />
</div>

{#each groups as g (g.name)}
  {@const isCollapsed = collapsed.has(g.name) && !query}
  <div class="border-t border-border first:border-t-0">
    <button
      type="button"
      class="flex w-full items-center gap-2 py-3"
      onclick={() => (collapsed.has(g.name) ? collapsed.delete(g.name) : collapsed.add(g.name))}
    >
      <ChevronDown class={['size-4 text-muted-foreground transition-transform', isCollapsed && '-rotate-90']} />
      <h3 class="text-sm font-semibold tracking-tight">{g.name}</h3>
    </button>
    {#if !isCollapsed}
      <div class="flex flex-wrap gap-2 pb-3">
        {#each g.options as o (o.value)}
          {@const excluded = st.exclude.includes(o.value)}
          {@const included = st.include.includes(o.value)}
          <button
            type="button"
            onclick={() => onToggle(o.value)}
            title={pillTitle(included, excluded, excludable)}
            class={pillClass(included || excluded, excluded, 'px-3 py-1.5 text-sm')}
          >
            {o.label}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/each}
