<script lang="ts">
  import { visibleEngagementGroups } from '#lib/facets.js';
  import { appliesWorkModeRemoteAnywhere, workModeRemoteLabel } from '#lib/filters.js';
  import type { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import type { FacetCounts } from '#lib/types.js';
  import ChipFacet from './ChipFacet.svelte';

  // The filter modal's Work pane: work format (with the remote-anywhere switch where it
  // changes the answer), employment type and the contract terms, over the staged filters.
  let { store, counts }: { store: StagedFilters; counts: FacetCounts | null } = $props();

  const engagementSel = $derived(store.facet('engagement'));
  const engagementGroups = $derived(
    visibleEngagementGroups(counts?.facets?.engagement, [...engagementSel.include, ...engagementSel.exclude]),
  );
</script>

<ChipFacet {store} param="work_mode" label="Work format" {counts} />
{#if appliesWorkModeRemoteAnywhere(store.value)}
  <!-- Only shown when it changes the answer (remote + a physical format + a
       location facet): a toggle that does nothing reads as broken. -->
  <label class="mt-3 flex cursor-pointer items-center gap-2 text-sm">
    <input
      type="checkbox"
      class="size-4 rounded border-border"
      checked={store.value.workModeRemoteAnywhere}
      onchange={(e) => store.setWorkModeRemoteAnywhere(e.currentTarget.checked)}
    />
    <span>{workModeRemoteLabel(store.value)}</span>
  </label>
  <p class="mt-1 text-xs text-muted-foreground">
    Hybrid and on-site stay within the locations above; remote work is not bound by them.
  </p>
{/if}
<div class="mt-6"><ChipFacet {store} param="employment_type" label="Employment type" {counts} /></div>
<!-- The legal terms a posting states (internal/dict/engagement), one block per market
     whose term it is, and only the markets the current scope reaches. -->
{#if engagementGroups.length > 0}
  <div class="mt-6">
    <h3 class="text-sm font-medium">Contract terms</h3>
    <p class="mt-1 text-xs text-muted-foreground">
      Employee or own business, in each market's own terms — as the posting states it.
    </p>
    {#each engagementGroups as g (g.market)}
      <div class="mt-4">
        <ChipFacet {store} param="engagement" label={g.market} options={g.options} {counts} />
      </div>
    {/each}
  </div>
{/if}
