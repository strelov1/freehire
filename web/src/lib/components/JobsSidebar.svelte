<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { FilterStore } from '#lib/filters.js';
  import FilterSummary from './filters/FilterSummary.svelte';

  // The feed's desktop filter sidebar: the caller's `top`, then the filter summary.
  let {
    top,
    filters,
    exclude,
    onOpen,
    canSave,
    matchAvailable,
  }: {
    top: Snippet | undefined;
    filters: FilterStore;
    exclude: string[];
    onOpen: () => void;
    canSave: boolean;
    matchAvailable: boolean;
  } = $props();
</script>

<aside class="hidden w-72 shrink-0 md:block">
  <!-- `top-20` is the site header's own `h-14` plus 24px of air. At `top-6` the rail
       pinned 24px from the VIEWPORT, so the opaque header (`sticky top-0`) covered its
       first 32px — here, the "Filters" heading — for the whole scroll. The max-height
       is then what is left of the viewport, less the same 24px at the bottom. -->
  <div class="sticky top-20 flex max-h-[calc(100vh-6.5rem)] flex-col gap-4 overflow-y-auto">
    {@render top?.()}
    <!-- Card chrome only once there are filters to list: around one "All filters" button it
         was a framed box holding nothing. -->
    <div class={filters.active > 0 ? 'rounded-xl border border-border bg-card p-4' : 'px-1'}>
      <FilterSummary store={filters} {exclude} {onOpen} {canSave} {matchAvailable} />
    </div>
  </div>
</aside>
