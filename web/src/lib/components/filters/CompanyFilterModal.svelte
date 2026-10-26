<script lang="ts">
  import { COMPANY_FACETS } from '#lib/facets.js';
  import { railFacetSearchItems, type FilterSearchHit } from '#lib/filterSearch.js';
  import type { CompanyFilterStore } from '#lib/companyFilters.js';
  import { StagedCompanyFilters } from '#lib/stagedCompanyFilters.svelte.js';
  import { COMPANY_RAIL_GROUPS, type RailEntry, type RailSection } from '#lib/filterSections.js';
  import type { FacetCounts } from '#lib/types.js';
  import FacetSection from '../facets/FacetSection.svelte';
  import FilterModalShell from './FilterModalShell.svelte';

  // The companies-catalog filter modal: a thin wrapper over FilterModalShell, the
  // company counterpart of FilterModal. All company facets are plain include-only
  // controls, so the rail is a single `FILTERS` section and the footer just applies
  // (no per-facet count endpoint to preview against).
  //
  // The rail is a presentation grouping over COMPANY_FACETS, not 1:1 with it (mirroring
  // the job modal's RAIL): related facets fold into one pane that stacks their
  // FacetSections, so the closely-linked geography, company-shape, and YC-directory
  // facets read as three panes instead of scattering across seven rail rows. Each param
  // stays a distinct query param — only the presentation is grouped.
  let { store, open = false, onClose }: { store: CompanyFilterStore; open?: boolean; onClose: () => void } = $props();

  const staged = new StagedCompanyFilters();

  // Rail groups live in filterSections.ts beside the job modal's RAIL, where a test
  // can assert they cover every COMPANY_FACETS param — an ungrouped facet is not
  // merely misplaced, it is unreachable.
  const rail: RailEntry[] = COMPANY_RAIL_GROUPS.map((g) => ({
    key: g.key,
    label: g.label,
    section: 'FILTERS',
    kind: 'facet',
  }));
  const sections: RailSection[] = ['FILTERS'];

  const paramsOf = (key: string) => COMPANY_RAIL_GROUPS.find((g) => g.key === key)?.params ?? [];

  // Company facets are include-only, so the badge is the total selected across the
  // group's params.
  function entryCount(e: RailEntry): number {
    return paramsOf(e.key).reduce((n, p) => n + staged.facet(p).include.length, 0);
  }

  function seed() {
    staged.seed(store.value);
  }

  function apply() {
    staged.commit(store);
  }

  const searchItems = railFacetSearchItems(COMPANY_FACETS, COMPANY_RAIL_GROUPS);

  function searchSelection(hit: FilterSearchHit) {
    if (!hit.param || hit.value == null) return { included: false, excluded: false, excludable: false };
    return {
      included: staged.facet(hit.param).include.includes(hit.value),
      excluded: false,
      excludable: false,
    };
  }

  function onSearchToggle(hit: FilterSearchHit) {
    if (hit.param && hit.value != null) staged.pick(hit.param, hit.value);
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
  {searchItems}
  {onSearchToggle}
  {searchSelection}
/>

{#snippet pane(entry: RailEntry, _live: FacetCounts | null)}
  {@const params = paramsOf(entry.key)}
  <div class="space-y-4">
    {#each params as param (param)}
      {@const def = COMPANY_FACETS.find((d) => d.param === param)}
      {#if def}<FacetSection {def} store={staged} expand />{/if}
    {/each}
  </div>
{/snippet}
