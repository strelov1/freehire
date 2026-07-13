<script lang="ts">
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { ChevronDown, Globe, House, Blend, Building } from '@lucide/svelte';
  import { WORK_MODE_OPTIONS, REGION_OPTIONS, type FacetStore, type FacetOption } from '$lib/facets';
  import type { FacetCounts } from '$lib/types';
  import { summarizeScope, JOBS_SCOPE, COMPANIES_SCOPE } from '$lib/headerScope';
  import { pillClass, pillTitle } from './facets/pill';
  import LocationPane from './filters/LocationPane.svelte';

  // The header's cross-cutting location filter — a scope-prefix trigger inside the
  // search box that opens a popover, in one of three modes:
  //   • jobs      — work-format pills + the full LocationPane, live-filtering the feed
  //   • companies — Region + Remote-hiring pills, live-filtering the company list
  //   • launcher  — listless pages: pills navigate to the jobs feed with that scope
  // jobs/companies drive the page's existing FacetStore (store.cycle/clearFacet), so
  // there is no new filter logic; launcher has no store and jumps to /jobs instead.
  let {
    variant = 'jobs',
    store,
    counts = null,
  }: {
    variant?: 'jobs' | 'companies' | 'launcher';
    store?: FacetStore;
    counts?: FacetCounts | null;
  } = $props();

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  // Launcher has no persisted selection, so its trigger stays the neutral label.
  const summary = $derived(
    store
      ? summarizeScope(store, variant === 'companies' ? COMPANIES_SCOPE : JOBS_SCOPE)
      : { icon: 'globe' as const, label: 'Location' },
  );
  const ICONS = { globe: Globe, remote: House, hybrid: Blend, onsite: Building } as const;
  const TriggerIcon = $derived(ICONS[summary.icon]);

  // Facets "Clear all" resets, per mode.
  const CLEAR_PARAMS = {
    jobs: ['work_mode', 'regions', 'countries', 'cities'],
    companies: ['regions', 'remote_regions'],
    launcher: [],
  } as const;
  function clearAll() {
    if (!store) return;
    for (const p of CLEAR_PARAMS[variant]) store.clearFacet(p);
  }

  // Launcher pick: no list to filter in place, so land on the jobs feed with the
  // chosen facet applied (further tweaks are live there).
  function launch(param: string, value: string) {
    open = false;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- one facet appended to the resolved feed path
    void goto(`${resolve('/')}?${param}=${encodeURIComponent(value)}`);
  }

  function onWindowClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) open = false;
  }
  afterNavigate(() => {
    open = false;
  });
</script>

<svelte:window onclick={onWindowClick} onkeydown={(e) => e.key === 'Escape' && (open = false)} />

{#snippet storePills(param: string, options: FacetOption[])}
  {@const f = store!.facet(param)}
  <div class="flex flex-wrap gap-2">
    {#each options as opt (opt.value)}
      {@const inc = f.include.includes(opt.value)}
      {@const exc = f.exclude.includes(opt.value)}
      <button
        type="button"
        onclick={() => store!.cycle(param, opt.value)}
        title={pillTitle(inc, exc, true)}
        class={pillClass(inc || exc, exc, 'px-3 py-1.5 text-sm')}
      >
        {opt.label}
      </button>
    {/each}
  </div>
{/snippet}

{#snippet navPills(param: string, options: FacetOption[])}
  <div class="flex flex-wrap gap-2">
    {#each options as opt (opt.value)}
      <button
        type="button"
        onclick={() => launch(param, opt.value)}
        class={pillClass(false, false, 'px-3 py-1.5 text-sm')}
      >
        {opt.label}
      </button>
    {/each}
  </div>
{/snippet}

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
        <span class="text-sm font-semibold tracking-tight">
          {variant === 'companies' ? 'Location' : 'Location & format'}
        </span>
        {#if variant !== 'launcher'}
          <button
            type="button"
            onclick={clearAll}
            class="text-xs text-muted-foreground transition-colors hover:text-foreground"
          >
            Clear all
          </button>
        {/if}
      </div>

      {#if variant === 'jobs'}
        <div class="mb-4">
          <div class="mb-2 text-sm font-semibold tracking-tight">Work format</div>
          {@render storePills('work_mode', WORK_MODE_OPTIONS)}
        </div>
        <div class="border-t border-border pt-4">
          <LocationPane store={store!} {counts} />
        </div>
      {:else if variant === 'companies'}
        <div class="mb-4">
          <div class="mb-2 text-sm font-semibold tracking-tight">Region</div>
          {@render storePills('regions', REGION_OPTIONS)}
        </div>
        <div>
          <div class="mb-2 text-sm font-semibold tracking-tight">Remote hiring</div>
          {@render storePills('remote_regions', REGION_OPTIONS)}
        </div>
      {:else}
        <div class="mb-4">
          <div class="mb-2 text-sm font-semibold tracking-tight">Work format</div>
          {@render navPills('work_mode', WORK_MODE_OPTIONS)}
        </div>
        <div>
          <div class="mb-2 text-sm font-semibold tracking-tight">Region</div>
          {@render navPills('regions', REGION_OPTIONS)}
        </div>
      {/if}
    </div>
  {/if}
</div>
