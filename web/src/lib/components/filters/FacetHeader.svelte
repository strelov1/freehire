<script lang="ts">
  import { X } from '@lucide/svelte';
  import { FACETS, type FacetStore } from '$lib/facets';
  import { cn } from '$lib/utils';

  // A single-param facet header: the label plus the Exclude toggle (when the facet is
  // excludable and has a selection) and a Clear button. Shared by the chip facets and
  // the Specialization pane so the per-facet controls read identically everywhere.
  // `noExclude` forces the Exclude toggle off for a plain-select reuse (e.g. the profile
  // editor, where a value is a plain choice, not a filter to exclude).
  let { store, param, label, noExclude = false }: { store: FacetStore; param: string; label: string; noExclude?: boolean } = $props();

  const excludable = $derived(!noExclude && (FACETS.find((d) => d.param === param)?.excludable ?? false));
  const st = $derived(store.facet(param));
</script>

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
