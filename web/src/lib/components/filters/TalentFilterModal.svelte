<script lang="ts">
  import { api } from '$lib/api';
  import { TALENT_FACETS } from '$lib/facets';
  import type { TalentFilterStore } from '$lib/talentFilters';
  import { YEAR_THRESHOLDS } from '$lib/talentFacetModel';
  import { StagedTalentFilters } from '$lib/stagedTalentFilters.svelte';
  import { TALENT_RAIL_GROUPS, type RailEntry, type RailSection } from '$lib/filterSections';
  import type { FacetCounts } from '$lib/types';
  import { Chip } from '$lib/ui';
  import FacetSection from '../facets/FacetSection.svelte';
  import FilterModalShell from './FilterModalShell.svelte';

  // The Talent Network catalogue's filter modal: a thin wrapper over FilterModalShell,
  // the catalogue counterpart of FilterModal and CompanyFilterModal. The shell owns the
  // chrome — the rail, the footer, the deferred apply — and this file owns only what is
  // catalogue-specific.
  //
  // Every catalogue facet is include-only, so the rail is one `FILTERS` section and the
  // panes are plain FacetSections. The open vocabularies (skills, city) are declared
  // `dynamic` in TALENT_FACETS, so the shell's live counts drive them: they arrive
  // searchable, with the number of members behind each value, which is the whole reason
  // this modal exists rather than another row of pills.
  let {
    store,
    open = false,
    onClose,
  }: { store: TalentFilterStore; open?: boolean; onClose: () => void } = $props();

  const staged = new StagedTalentFilters();

  // Rail groups live in filterSections.ts beside the job and company rails, where a test
  // asserts they cover every TALENT_FACETS param — an ungrouped facet is not merely
  // misplaced, it is unreachable, which is the exact failure this feature was built to
  // fix.
  const rail: RailEntry[] = TALENT_RAIL_GROUPS.map((g) => ({
    key: g.key,
    label: g.label,
    section: 'FILTERS',
    kind: 'facet',
  }));
  const sections: RailSection[] = ['FILTERS'];

  const paramsOf = (key: string) => TALENT_RAIL_GROUPS.find((g) => g.key === key)?.params ?? [];

  // Include-only, so a pane's badge is the total selected across its params. Experience
  // is not a facet and is counted separately — it rides in the Grade pane because it
  // answers the same question in a different unit.
  function entryCount(e: RailEntry): number {
    const facets = paramsOf(e.key).reduce((n, p) => n + staged.facet(p).include.length, 0);
    return e.key === 'seniorities' && staged.value.minYears ? facets + 1 : facets;
  }

  function seed() {
    staged.seed(store.value);
  }

  function apply() {
    staged.commit(store);
  }

  /** The shell hands over the STAGED parameters, so every pane previews the filter being
   *  edited rather than the one currently applied — which is the difference between a
   *  count that helps you choose and one that describes where you already are. */
  async function countsFetch(params: URLSearchParams): Promise<FacetCounts> {
    return api.talentFacets(params.toString());
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
  {countsFetch}
  applyLabel="Show candidates"
  {pane}
/>

{#snippet pane(entry: RailEntry, live: FacetCounts | null)}
  {@const params = paramsOf(entry.key)}
  <div class="space-y-4">
    {#each params as param (param)}
      {@const def = TALENT_FACETS.find((d) => d.param === param)}
      {#if def}<FacetSection {def} store={staged} counts={live} expand />{/if}
    {/each}

    {#if entry.key === 'seniorities'}
      <!-- Years rides in the Grade pane: it answers the same question — how far along is
      this person — in a different unit, and a pane of its own would make a visitor choose
      between two spellings of one idea.

      Single-select, because it is a THRESHOLD and not a set: clicking the active one
      clears it. -->
      <div class="flex flex-col gap-1.5">
        <h3 class="text-xs font-medium text-muted-foreground">Experience</h3>
        <div class="flex flex-wrap gap-1.5">
          {#each YEAR_THRESHOLDS as years (years)}
            {@const selected = staged.value.minYears === years}
            <button type="button" onclick={() => staged.setMinYears(selected ? undefined : years)}>
              <Chip variant={selected ? 'primary' : 'default'}>{years}+ years</Chip>
            </button>
          {/each}
        </div>
      </div>
    {/if}
  </div>
{/snippet}
