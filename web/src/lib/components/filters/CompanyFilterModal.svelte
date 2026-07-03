<script lang="ts">
  import { COMPANY_FACETS } from '$lib/facets';
  import type { CompanyFilterStore } from '$lib/companyFilters';
  import { StagedCompanyFilters } from '$lib/stagedCompanyFilters.svelte';
  import type { RailEntry, RailSection } from '$lib/filterSections';
  import FacetSection from '../facets/FacetSection.svelte';
  import FilterModalShell from './FilterModalShell.svelte';

  // The companies-catalog filter modal: a thin wrapper over FilterModalShell, the
  // company counterpart of FilterModal. All company facets are plain include-only
  // controls, so the rail is a single `FILTERS` section of FacetSection panes and the
  // footer just applies (no per-facet count endpoint to preview against).
  let { store, open = false, onClose }: { store: CompanyFilterStore; open?: boolean; onClose: () => void } = $props();

  const staged = new StagedCompanyFilters();

  const rail: RailEntry[] = COMPANY_FACETS.map((def) => ({
    key: def.param,
    label: def.label,
    section: 'FILTERS',
    kind: 'facet',
    facetParam: def.param,
  }));
  const sections: RailSection[] = ['FILTERS'];

  // Company facets are include-only, so the badge is just the selected count.
  function entryCount(e: RailEntry): number {
    return staged.facet(e.facetParam ?? e.key).include.length;
  }

  function seed() {
    staged.seed(store.value);
  }

  function apply() {
    staged.commit(store);
  }
</script>

<FilterModalShell
  {open}
  {onClose}
  title="All filters"
  {rail}
  {sections}
  {staged}
  {entryCount}
  {seed}
  {apply}
  applyLabel="Show results"
  {pane}
/>

{#snippet pane(entry: RailEntry)}
  {@const def = COMPANY_FACETS.find((d) => d.param === entry.facetParam)}
  {#if def}<FacetSection {def} store={staged} expand />{/if}
{/snippet}
