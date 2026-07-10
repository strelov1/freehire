<script lang="ts">
  import { COMPANY_FACETS } from '$lib/facets';
  import type { CompanyFilterStore } from '$lib/companyFilters';
  import { StagedCompanyFilters } from '$lib/stagedCompanyFilters.svelte';
  import type { RailEntry, RailSection } from '$lib/filterSections';
  import type { FacetCounts } from '$lib/types';
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

  // Rail groups: each pane renders the FacetSections for its `params`, in order. A
  // single-facet group is just its own pane (Collection/Country/Industry). Every
  // COMPANY_FACETS param appears in exactly one group.
  const GROUPS: { key: string; label: string; params: string[] }[] = [
    { key: 'collections', label: 'Collection', params: ['collections'] },
    { key: 'region', label: 'Region', params: ['regions', 'remote_regions'] },
    { key: 'countries', label: 'Country', params: ['countries'] },
    { key: 'domains', label: 'Industry', params: ['domains'] },
    { key: 'company', label: 'Company', params: ['company_type', 'company_size'] },
    { key: 'yc', label: 'Y Combinator', params: ['yc_status', 'yc_stage', 'yc_flags', 'yc_batch'] },
  ];

  const rail: RailEntry[] = GROUPS.map((g) => ({
    key: g.key,
    label: g.label,
    section: 'FILTERS',
    kind: 'facet',
  }));
  const sections: RailSection[] = ['FILTERS'];

  const paramsOf = (key: string) => GROUPS.find((g) => g.key === key)?.params ?? [];

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

{#snippet pane(entry: RailEntry, _live: FacetCounts | null)}
  {@const params = paramsOf(entry.key)}
  <div class="space-y-4">
    {#each params as param (param)}
      {@const def = COMPANY_FACETS.find((d) => d.param === param)}
      {#if def}<FacetSection {def} store={staged} expand />{/if}
    {/each}
  </div>
{/snippet}
