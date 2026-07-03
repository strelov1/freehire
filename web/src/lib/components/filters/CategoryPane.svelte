<script lang="ts">
  import { ChevronDown, Search, X } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import type { FacetStore } from '$lib/facets';
  import { CATEGORY_GROUPS } from '$lib/filterSections';
  import { cn } from '$lib/utils';
  import { pillClass } from '../facets/pill';

  // Specialization pane: category chips grouped into collapsible sections, with a
  // facet-local search filtering the visible options. Toggles the `category` facet.
  let { store }: { store: FacetStore } = $props();

  let query = $state('');
  const collapsed = new SvelteSet<string>();
  const st = $derived(store.facet('category'));
  const selected = $derived(st.values);

  const groups = $derived.by(() => {
    const q = query.trim().toLowerCase();
    return CATEGORY_GROUPS.map((g) => ({
      ...g,
      options: q ? g.options.filter((o) => o.label.toLowerCase().includes(q)) : g.options,
    })).filter((g) => g.options.length > 0);
  });
</script>

<div class="mb-2 flex min-h-6 items-center justify-between gap-2">
  <h3 class="text-sm font-semibold tracking-tight">Specialization</h3>
  {#if selected.length > 0}
    <div class="flex items-center gap-1">
      <button
        type="button"
        onclick={() => store.setExclude('category', !st.exclude)}
        title="Hide jobs that match the selected specializations"
        class={cn(
          'rounded-full px-2 py-0.5 text-xs font-medium transition-colors',
          st.exclude ? 'bg-destructive/15 text-destructive' : 'text-muted-foreground hover:text-foreground',
        )}
      >
        {st.exclude ? 'Excluding' : 'Exclude'}
      </button>
      <button
        type="button"
        onclick={() => store.clearFacet('category')}
        title="Clear Specialization"
        aria-label="Clear Specialization"
        class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-3.5" />
      </button>
    </div>
  {/if}
</div>

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
          <button
            type="button"
            onclick={() => store.toggle('category', o.value)}
            class={pillClass(selected.includes(o.value), st.exclude, 'px-3 py-1.5 text-sm')}
          >
            {o.label}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/each}
