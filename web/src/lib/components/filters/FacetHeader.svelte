<script lang="ts">
  import { X } from '@lucide/svelte';
  import type { FacetStore } from '$lib/facets';

  // A single-param facet header: the label plus a Clear button (shown once the facet
  // has a selection). Shared by the chip facets and the Specialization pane so the
  // per-facet header reads identically everywhere. Include/exclude is chosen per
  // value on the pills themselves, so there is no facet-wide exclude toggle here.
  let { store, param, label }: { store: FacetStore; param: string; label: string } = $props();

  const st = $derived(store.facet(param));
  const total = $derived(st.include.length + st.exclude.length);
</script>

<div class="mb-2 flex min-h-6 items-center justify-between gap-2">
  <h3 class="text-sm font-semibold tracking-tight">{label}</h3>
  {#if total > 0}
    <button
      type="button"
      onclick={() => store.clearFacet(param)}
      title="Clear {label}"
      aria-label="Clear {label}"
      class="flex size-5 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
    >
      <X class="size-3.5" />
    </button>
  {/if}
</div>
