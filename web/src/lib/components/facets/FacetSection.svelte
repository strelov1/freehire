<script lang="ts">
  import { X } from '@lucide/svelte';
  import { dynamicLabel, type FacetDef, type FacetOption, type FacetStore } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import PillGroup from './PillGroup.svelte';
  import RemoteSearchSelect from './RemoteSearchSelect.svelte';
  import SearchSelect from './SearchSelect.svelte';
  import TokenInput from './TokenInput.svelte';

  // One facet section: header (label, optional Any/All match toggle, Clear) plus the
  // control the facet declares. Each value is off / included / excluded; clicking a
  // control cycles an excludable facet (off → include → exclude → off) or plainly
  // toggles a non-excludable one (off ↔ include). All state lives in the store, keyed
  // by the facet's param. `counts` carries the live facet distribution that drives
  // dynamic (open-vocabulary) selects.
  let { def, store, counts, expand = false }: { def: FacetDef; store: FacetStore; counts?: FacetCounts | null; expand?: boolean } = $props();

  const st = $derived(store.facet(def.param));
  const total = $derived(st.include.length + st.exclude.length);
  // Excludable facets cycle through the exclude state; the rest only toggle include.
  const onToggle = (v: string) => (def.excludable ? store.cycle(def.param, v) : store.pick(def.param, v));

  // Options for a select facet: static for closed vocabularies, or built from the
  // live distribution (value → count, busiest first) for dynamic ones. Selected
  // values absent from the current distribution are still listed so they stay
  // removable.
  const selectOptions = $derived.by((): FacetOption[] => {
    if (!def.dynamic) return def.options ?? [];
    const dist = counts?.facets?.[def.param] ?? {};
    const keys = new Set<string>([...Object.keys(dist), ...st.include, ...st.exclude]);
    return [...keys]
      .map((value) => ({ value, label: dynamicLabel(def.param, value), count: dist[value] ?? 0 }))
      .toSorted((a, b) => b.count - a.count || a.label.localeCompare(b.label));
  });
  // The match/clear actions only appear once something is selected — so their
  // meaning is clear ("you picked these — match all, or clear them") rather than
  // abstract controls on an empty section. Match-all applies to the include set.
  const showMatch = $derived(def.hasAndOr && st.include.length > 1);
</script>

<div class="border-b border-border pb-4">
  <div class="mb-2 flex min-h-6 items-center justify-between gap-2">
    <div class="flex items-center gap-2">
      <h3 class="text-sm font-semibold tracking-tight">{def.label}</h3>
      {#if showMatch}
        <button
          type="button"
          onclick={() => store.setMatchAll(def.param, !st.matchAll)}
          title="Match any of / all of the included values"
          class="rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Match: {st.matchAll ? 'All' : 'Any'}
        </button>
      {/if}
    </div>
    {#if total > 0}
      <button
        type="button"
        onclick={() => store.clearFacet(def.param)}
        title="Clear {def.label}"
        aria-label="Clear {def.label}"
        class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      >
        <X class="size-3.5" />
      </button>
    {/if}
  </div>

  {#if def.control === 'pills'}
    <PillGroup
      options={def.options ?? []}
      include={st.include}
      exclude={st.exclude}
      excludable={def.excludable}
      {onToggle}
    />
  {:else if def.control === 'select'}
    <SearchSelect
      options={selectOptions}
      include={st.include}
      exclude={st.exclude}
      excludable={def.excludable}
      placeholder={def.placeholder}
      cap={def.cap}
      {onToggle}
      {expand}
    />
  {:else if def.control === 'remote' && def.remote}
    <RemoteSearchSelect
      search={def.remote}
      include={st.include}
      exclude={st.exclude}
      placeholder={def.placeholder}
      {onToggle}
      fallbackLabel={(v) => dynamicLabel(def.param, v)}
      {expand}
    />
  {:else}
    <TokenInput
      tokens={st.include}
      onAdd={(v) => store.add(def.param, v)}
      onRemove={(v) => store.remove(def.param, v)}
      placeholder={def.placeholder}
    />
  {/if}
</div>
