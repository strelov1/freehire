<script lang="ts">
  import { COMPANY_FACETS, type FacetDef } from '$lib/facets';
  import type { CompanyFilterStore } from '$lib/companyFilters';
  import FacetSection from './facets/FacetSection.svelte';

  // Pure presentation over the company filter store: iterate the company facet
  // registry and render each section. No live counts (the companies list is plain
  // SQL, not a faceted search) and no special controls — companies filter purely by
  // their denormalized facet arrays.
  let { store }: { store: CompanyFilterStore } = $props();

  const isActive = (param: string) => store.facet(param).values.length > 0;

  // Facets with a current selection float to the top; the rest keep registry order.
  // Keyed by param so Svelte moves the node rather than re-creating it (an open
  // select survives the reorder).
  const orderedFacets = $derived.by((): FacetDef[] => [
    ...COMPANY_FACETS.filter((d) => isActive(d.param)),
    ...COMPANY_FACETS.filter((d) => !isActive(d.param)),
  ]);
</script>

<div class="flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-base font-semibold tracking-tight">Filters</h2>
    {#if store.active > 0}
      <button type="button" class="text-xs text-muted-foreground transition-colors hover:text-foreground" onclick={() => store.clear()}>
        Reset all
      </button>
    {/if}
  </div>

  {#each orderedFacets as def (def.param)}
    <FacetSection {def} {store} />
  {/each}
</div>
