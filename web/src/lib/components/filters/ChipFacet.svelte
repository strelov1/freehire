<script lang="ts">
  import { X } from '@lucide/svelte';
  import { FACETS, type FacetOption, type FacetStore } from '$lib/facets';
  import { cn } from '$lib/utils';
  import PillGroup from '../facets/PillGroup.svelte';

  // One chip facet inside a modal pane: a header (label + optional Exclude toggle +
  // Clear) over a PillGroup — the same per-facet controls the sidebar's FacetSection
  // offers, so the merged/custom panes keep exclude + reset per facet. `excludable`
  // is read from the facet registry so it matches the sidebar exactly.
  let { store, param, label, options }: { store: FacetStore; param: string; label: string; options: FacetOption[] } = $props();

  const st = $derived(store.facet(param));
  const excludable = FACETS.find((d) => d.param === param)?.excludable ?? false;
</script>

<div>
  <div class="mb-2 flex min-h-6 items-center justify-between gap-2">
    <h3 class="text-sm font-semibold tracking-tight">{label}</h3>
    {#if st.values.length > 0}
      <div class="flex items-center gap-1">
        {#if excludable}
          <button
            type="button"
            onclick={() => store.setExclude(param, !st.exclude)}
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
          onclick={() => store.clearFacet(param)}
          title="Clear {label}"
          aria-label="Clear {label}"
          class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
        >
          <X class="size-3.5" />
        </button>
      </div>
    {/if}
  </div>
  <PillGroup {options} selected={st.values} exclude={st.exclude} onToggle={(v) => store.toggle(param, v)} />
</div>
