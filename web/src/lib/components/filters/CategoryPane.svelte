<script lang="ts">
  import type { Snippet } from 'svelte';
  import { ChevronDown, Search } from '@lucide/svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import type { FacetStore } from '#lib/facets.js';
  import type { FacetCounts } from '#lib/types.js';
  import { searchCategoryGroups } from '#lib/filterSections.js';
  import FacetHeader from './FacetHeader.svelte';
  import { pillClass, pillPressed, pillTitle } from '../facets/pill';
  import PillMeta from '../facets/PillMeta.svelte';

  // Specialization pane: category chips grouped into collapsible sections, with a
  // facet-local search filtering the visible options. In the job filter it toggles
  // the excludable `category` facet (each chip cycles off → include → exclude → off);
  // `plain` makes it an include-only choice (e.g. a profile category is not a filter
  // to exclude). When `counts` is passed, each chip shows its live match count.
  // `query` is bindable so a page can seed the search; `empty`, when given, renders
  // in place of the groups once the search matches no specialization. `hintSearch`
  // briefly rings the search field, pointing the eye at it from the modal-wide search.
  let {
    store,
    plain = false,
    counts = null,
    query = $bindable(''),
    empty,
    hintSearch = false,
  }: {
    store: FacetStore;
    plain?: boolean;
    counts?: FacetCounts | null;
    query?: string;
    empty?: Snippet<[string]>;
    hintSearch?: boolean;
  } = $props();

  let hinted = $state(false);
  $effect(() => {
    if (!hintSearch) return;
    hinted = true;
    const timer = setTimeout(() => (hinted = false), 1000);
    return () => clearTimeout(timer);
  });

  const dist = $derived(counts?.facets?.category ?? null);

  const excludable = $derived(!plain);
  const collapsed = new SvelteSet<string>();
  const st = $derived(store.facet('category'));
  const onToggle = (v: string) => (excludable ? store.cycle('category', v) : store.pick('category', v));

  const groups = $derived(searchCategoryGroups(query));
</script>

<FacetHeader {store} param="category" label="Specialization" />

<div
  class={[
    'mb-4 flex items-center gap-2 rounded-lg border px-3 transition-shadow duration-700 focus-within:ring-2 focus-within:ring-ring',
    hinted ? 'border-brand-ring ring-2 ring-brand-ring' : 'border-input',
  ]}
>
  <Search class="size-4 shrink-0 text-muted-foreground" />
  <input
    bind:value={query}
    placeholder="Search specializations…"
    aria-label="Search specializations"
    class="h-9 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
  />
</div>

{#if empty && query.trim() && groups.length === 0}
  {@render empty(query.trim())}
{/if}

{#each groups as g (g.name)}
  {@const isCollapsed = collapsed.has(g.name) && !query}
  <div class="border-t border-border first:border-t-0">
    <!-- The button sits inside the heading, not around it: a button's content may not
         hold a heading, and this way the group is still reachable by heading navigation
         while the button reports its state through aria-expanded. -->
    <h3 class="text-sm font-semibold tracking-tight">
      <button
        type="button"
        class="flex w-full items-center gap-2 py-3"
        aria-expanded={!isCollapsed}
        onclick={() => (collapsed.has(g.name) ? collapsed.delete(g.name) : collapsed.add(g.name))}
      >
        <ChevronDown class={['size-4 text-muted-foreground transition-transform', isCollapsed && '-rotate-90']} aria-hidden="true" />
        {g.name}
      </button>
    </h3>
    {#if !isCollapsed}
      <div class="flex flex-wrap gap-2 pb-3">
        {#each g.options as o (o.value)}
          {@const excluded = st.exclude.includes(o.value)}
          {@const included = st.include.includes(o.value)}
          <button
            type="button"
            onclick={() => onToggle(o.value)}
            aria-pressed={pillPressed(included, excluded)}
            title={pillTitle(included, excluded, excludable)}
            class={pillClass(included || excluded, excluded, 'px-3 py-1.5 text-sm')}
          >
            {o.label}<PillMeta {included} {excluded} count={dist ? (dist[o.value] ?? 0).toLocaleString() : undefined} />
          </button>
        {/each}
      </div>
    {/if}
  </div>
{/each}
