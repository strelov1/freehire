<script lang="ts">
  import { afterNavigate } from '$app/navigation';
  import { ChevronDown, Globe, House, Blend, Building } from '@lucide/svelte';
  import { WORK_MODE_OPTIONS, type FacetStore } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import { summarizeScope } from '$lib/headerScope';
  import { pillClass, pillTitle } from './facets/pill';
  import LocationPane from './filters/LocationPane.svelte';

  // The header's Location & work-format quick-filter: a scope-prefix trigger inside
  // the search box that opens a popover of work-format pills + the full LocationPane.
  // Pure UI over the page's existing job-filter store — every control drives the same
  // facet API the FilterModal uses (store.cycle/clearFacet), so there is no new filter
  // logic here and selections still mirror to the URL/localStorage and reload the list.
  let { store, counts }: { store: FacetStore; counts: FacetCounts | null } = $props();

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const summary = $derived(summarizeScope(store));
  const ICONS = { globe: Globe, remote: House, hybrid: Blend, onsite: Building } as const;
  const TriggerIcon = $derived(ICONS[summary.icon]);

  const wm = $derived(store.facet('work_mode'));

  // Facets this popover owns, cleared together by "Clear all".
  const SCOPE_PARAMS = ['work_mode', 'regions', 'countries', 'cities'] as const;
  function clearAll() {
    for (const p of SCOPE_PARAMS) store.clearFacet(p);
  }

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }
  afterNavigate(() => {
    open = false;
  });
</script>

<svelte:window onclick={onWindowClick} onkeydown={(e) => e.key === 'Escape' && (open = false)} />

<div class="relative shrink-0" bind:this={root}>
  <button
    type="button"
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={(e) => {
      // Stop the toggle's own click from reaching the window outside-handler, which
      // would otherwise immediately re-close the just-opened popover.
      e.stopPropagation();
      open = !open;
    }}
    class="flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
  >
    <TriggerIcon class="size-4 shrink-0" />
    <span class="hidden max-w-40 truncate sm:inline">{summary.label}</span>
    <ChevronDown class="size-3.5 shrink-0 opacity-60" />
  </button>

  {#if open}
    <div
      role="menu"
      class="absolute left-0 top-[calc(100%+0.75rem)] z-50 max-h-[70vh] w-80 overflow-y-auto rounded-md border border-border bg-background p-4 shadow-lg"
    >
      <div class="mb-3 flex items-center justify-between">
        <span class="text-sm font-semibold tracking-tight">Location &amp; format</span>
        <button
          type="button"
          onclick={clearAll}
          class="text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          Clear all
        </button>
      </div>

      <div class="mb-4">
        <div class="mb-2 text-sm font-semibold tracking-tight">Work format</div>
        <div class="flex flex-wrap gap-2">
          {#each WORK_MODE_OPTIONS as opt (opt.value)}
            {@const inc = wm.include.includes(opt.value)}
            {@const exc = wm.exclude.includes(opt.value)}
            <button
              type="button"
              onclick={() => store.cycle('work_mode', opt.value)}
              title={pillTitle(inc, exc, true)}
              class={pillClass(inc || exc, exc, 'px-3 py-1.5 text-sm')}
            >
              {opt.label}
            </button>
          {/each}
        </div>
      </div>

      <div class="border-t border-border pt-4">
        <LocationPane {store} {counts} />
      </div>
    </div>
  {/if}
</div>
