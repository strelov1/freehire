<script lang="ts">
  import { EMPLOYER_CREDENTIALS } from '#lib/facets.js';
  import { SHORT_APPLICATION_MAX } from '#lib/jobFacts.js';
  import type { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import type { FacetCounts } from '#lib/types.js';
  import ChipFacet from './ChipFacet.svelte';

  // FilterModal's Relocation pane: relocation, employer credentials, visa, AI interviews,
  // auto-apply, short applications and security clearance, on the modal's staged filters
  // (`store`). `clearancePicked` is how many clearance levels are selected.
  let {
    store,
    counts,
    clearancePicked,
  }: { store: StagedFilters; counts: FacetCounts | null; clearancePicked: number } = $props();
</script>

<ChipFacet {store} param="relocation" label="Relocation" {counts} />
<div class="mt-6">
  <ChipFacet {store} param="collections" label="Employer credentials" {counts} options={EMPLOYER_CREDENTIALS} />
</div>
<h3 class="mb-2 mt-6 text-sm font-semibold tracking-tight">Visa</h3>
<label class="flex cursor-pointer items-center gap-2 text-sm">
  <input
    type="checkbox"
    class="size-4 rounded border-border"
    checked={store.value.visa}
    onchange={(e) => store.setVisa(e.currentTarget.checked)}
  />
  <span>Offers visa sponsorship</span>
</label>
<!-- Stated as what it hides, not as a judgement. The badge names the practice and
     its report count; this is only the way to act on it. -->
<label class="flex cursor-pointer items-center gap-2 text-sm">
  <input
    type="checkbox"
    class="size-4 rounded border-border"
    checked={store.value.hideAIInterview}
    onchange={(e) => store.setHideAIInterview(e.currentTarget.checked)}
  />
  <span>Hide employers reported to interview with AI</span>
</label>
<h3 class="mb-2 mt-6 text-sm font-semibold tracking-tight">Auto-apply</h3>
<label class="flex cursor-pointer items-center gap-2 text-sm">
  <input
    type="checkbox"
    class="size-4 rounded border-border"
    checked={store.value.autoApplyAvailable}
    onchange={(e) => store.setAutoApplyAvailable(e.currentTarget.checked)}
  />
  <span>Auto-apply available</span>
</label>
<p class="mt-1 text-xs text-muted-foreground">Best-effort — successful submission isn't guaranteed.</p>
<!-- Checked for ANY bound, not only the toggle's own: a link carrying
     apply_questions_max=5 still narrows the list, and a box reading unchecked over
     it would lie. Unchecking clears whichever bound it was. -->
<h3 class="mb-2 mt-6 text-sm font-semibold tracking-tight">Application</h3>
<label class="flex cursor-pointer items-center gap-2 text-sm">
  <input
    type="checkbox"
    class="size-4 rounded border-border"
    checked={store.value.applyQuestionsMax != null}
    onchange={(e) => store.setApplyQuestionsMax(e.currentTarget.checked ? SHORT_APPLICATION_MAX : null)}
  />
  <span>Short application</span>
</label>
<p class="mt-1 text-xs text-muted-foreground">
  {store.value.applyQuestionsMax ?? SHORT_APPLICATION_MAX} or fewer required questions beyond your CV and contact details;
  jobs whose form we haven't read are hidden.
</p>
<!-- The levels a posting states, not a yes/no: someone who cannot hold a clearance
     picks "No explicit reference", someone who holds Secret picks the rungs they
     can work under. The old Hide/Only toggle is this facet's `none` chip, included
     or excluded, and its links still read that way (facetModel.readLegacyClearance). -->
<h3 class="mb-2 mt-6 text-sm font-semibold tracking-tight">Security clearance</h3>
<ChipFacet {store} param="clearance_levels" label="Clearance level" {counts} />
<p class="mt-2 text-xs text-muted-foreground">
  {#if clearancePicked === 0}
    Showing every level — select one or more to narrow.
  {/if}
  Other covers non-US vetting (UK SC/DV, AU NV1, NATO) and clearances stated without a level.
</p>
