<script lang="ts">
  import { X } from '@lucide/svelte';
  import { dynamicLabel, type FacetDef, type FacetOption, type FacetStore } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import { cn } from '$lib/utils';
  import PillGroup from './PillGroup.svelte';
  import RemoteSearchSelect from './RemoteSearchSelect.svelte';
  import SearchSelect from './SearchSelect.svelte';
  import TokenInput from './TokenInput.svelte';

  // One facet section: header (label, optional Any/All match toggle, optional
  // exclude toggle) plus the control the facet declares. All state lives in the
  // store, keyed by the facet's param. `counts` carries the live facet
  // distribution that drives dynamic (open-vocabulary) selects.
  let { def, store, counts, expand = false }: { def: FacetDef; store: FacetStore; counts?: FacetCounts | null; expand?: boolean } = $props();

  const st = $derived(store.facet(def.param));

  // Options for a select facet: static for closed vocabularies, or built from the
  // live distribution (value → count, busiest first) for dynamic ones. Selected
  // values absent from the current distribution are still listed so they stay
  // removable.
  const selectOptions = $derived.by((): FacetOption[] => {
    if (!def.dynamic) return def.options ?? [];
    const dist = counts?.facets?.[def.param] ?? {};
    const keys = new Set<string>([...Object.keys(dist), ...st.values]);
    return [...keys]
      .map((value) => ({ value, label: dynamicLabel(def.param, value), count: dist[value] ?? 0 }))
      .toSorted((a, b) => b.count - a.count || a.label.localeCompare(b.label));
  });
  // The exclude/clear actions only appear once something is selected — so their
  // meaning is clear ("you picked these — hide them, or clear them") rather than
  // abstract controls on an empty section.
  const showMatch = $derived(def.hasAndOr && !st.exclude && st.values.length > 1);
</script>

<div class="border-b border-border pb-4">
  <div class="mb-2 flex min-h-6 items-center justify-between gap-2">
    <div class="flex items-center gap-2">
      <h3 class="text-sm font-semibold tracking-tight">{def.label}</h3>
      {#if showMatch}
        <button
          type="button"
          onclick={() => store.setMatchAll(def.param, !st.matchAll)}
          title="Match any of / all of the selected values"
          class="rounded-full border border-border px-2 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          Match: {st.matchAll ? 'All' : 'Any'}
        </button>
      {/if}
    </div>
    {#if st.values.length > 0}
      <div class="flex items-center gap-1">
        {#if def.excludable}
          <button
            type="button"
            onclick={() => store.setExclude(def.param, !st.exclude)}
            title="Hide jobs that match the selected options"
            class={cn(
              'rounded-full px-2 py-0.5 text-xs font-medium transition-colors',
              st.exclude ? 'bg-destructive/15 text-destructive' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {st.exclude ? 'Excluding' : 'Exclude'}
          </button>
        {/if}
        <button
          type="button"
          onclick={() => store.clearFacet(def.param)}
          title="Clear {def.label}"
          aria-label="Clear {def.label}"
          class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-3.5" />
        </button>
      </div>
    {/if}
  </div>

  {#if def.control === 'pills'}
    <PillGroup
      options={def.options ?? []}
      selected={st.values}
      exclude={st.exclude}
      onToggle={(v) => store.toggle(def.param, v)}
    />
  {:else if def.control === 'select'}
    <SearchSelect
      options={selectOptions}
      selected={st.values}
      exclude={st.exclude}
      placeholder={def.placeholder}
      onToggle={(v) => store.toggle(def.param, v)}
      {expand}
    />
  {:else if def.control === 'remote' && def.remote}
    <RemoteSearchSelect
      search={def.remote}
      selected={st.values}
      exclude={st.exclude}
      placeholder={def.placeholder}
      onToggle={(v) => store.toggle(def.param, v)}
      fallbackLabel={(v) => dynamicLabel(def.param, v)}
      {expand}
    />
  {:else}
    <TokenInput
      tokens={st.values}
      onAdd={(v) => store.add(def.param, v)}
      onRemove={(v) => store.remove(def.param, v)}
      placeholder={def.placeholder}
    />
  {/if}
</div>
