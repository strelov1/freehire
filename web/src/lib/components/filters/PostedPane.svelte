<script lang="ts">
  import { FRESHNESS_PRESETS, freshnessLabel } from '#lib/filterControls.js';
  import type { StagedFilters } from '#lib/stagedFilters.svelte.js';
  import type { FacetCounts } from '#lib/types.js';
  import ChipFacet from './ChipFacet.svelte';

  // The filter modal's Posted pane: hide viewed, the two date bounds and the posting's
  // reality, over the staged filters.
  let {
    store,
    counts,
    showHideSeen,
    openWithinAvailable,
  }: { store: StagedFilters; counts: FacetCounts | null; showHideSeen: boolean; openWithinAvailable: boolean } =
    $props();

  // A non-preset value (hand-edited URL) has no exact stop, so it reads as "Any"
  // (the rightmost stop) rather than snapping to "Today".
  const freshnessIndex = $derived.by(() => {
    const i = FRESHNESS_PRESETS.findIndex((p) => p.days === store.value.postedWithinDays);
    return i < 0 ? FRESHNESS_PRESETS.length - 1 : i;
  });

  // The first-seen bound rides the same stops and the same snap-to-Any rule.
  const openIndex = $derived.by(() => {
    const i = FRESHNESS_PRESETS.findIndex((p) => p.days === store.value.openWithinDays);
    return i < 0 ? FRESHNESS_PRESETS.length - 1 : i;
  });
</script>

{#if showHideSeen}
  <div class="mb-6">
    <label class="flex cursor-pointer items-center gap-2 text-sm">
      <input
        type="checkbox"
        class="size-4 rounded border-border"
        checked={store.value.hideSeen}
        onchange={(e) => store.setHideSeen(e.currentTarget.checked)}
      />
      <span>Hide viewed</span>
    </label>
    <p class="mt-1 text-xs text-muted-foreground">Skip jobs you have already opened.</p>
  </div>
{/if}
<!-- Two bounds over one set of stops, each labelled for WHOSE date it reads. They
     agree on most postings, which is exactly why the labels matter: where they
     disagree is where a board has restated its posting date, and that is the case
     the second bound was added for. `Open within` leads because it is the one no
     source can rewrite. -->
{#if openWithinAvailable}
  <div class="mb-2 flex items-center justify-between">
    <h3 class="text-sm font-semibold tracking-tight">Open within</h3>
    <span class="text-xs font-medium text-muted-foreground">{freshnessLabel(store.value.openWithinDays)}</span>
  </div>
  <input
    type="range"
    min="0"
    max={FRESHNESS_PRESETS.length - 1}
    step="1"
    value={openIndex}
    oninput={(e) => store.setOpenWithinDays(FRESHNESS_PRESETS[Number(e.currentTarget.value)]?.days ?? null)}
    aria-label="Open within"
    aria-valuetext={freshnessLabel(store.value.openWithinDays)}
    class="h-6 w-full accent-primary"
  />
  <p class="mt-2 text-xs text-muted-foreground">How long the posting has been here.</p>
{/if}
<div class="mb-2 flex items-center justify-between" class:mt-6={openWithinAvailable}>
  <h3 class="text-sm font-semibold tracking-tight">Posted within</h3>
  <span class="text-xs font-medium text-muted-foreground">{freshnessLabel(store.value.postedWithinDays)}</span>
</div>
<input
  type="range"
  min="0"
  max={FRESHNESS_PRESETS.length - 1}
  step="1"
  value={freshnessIndex}
  oninput={(e) => store.setPostedWithinDays(FRESHNESS_PRESETS[Number(e.currentTarget.value)]?.days ?? null)}
  aria-label="Posted within"
  aria-valuetext={freshnessLabel(store.value.postedWithinDays)}
  class="h-6 w-full accent-primary"
/>
{#if openWithinAvailable}
  <p class="mt-2 text-xs text-muted-foreground">The date the board states. Some boards restate it on every crawl.</p>
{/if}
<!-- Under the age bound, not in a pane of its own: the reality classes answer the
     same question the slider does, and the age is most of what makes a posting read
     as evergreen. -->
<div class="mt-6"><ChipFacet {store} param="reality" label="Posting reality" {counts} /></div>
<p class="mt-2 text-xs text-muted-foreground">Evergreen postings sit open permanently rather than for one opening.</p>
