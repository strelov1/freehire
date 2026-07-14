<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { cn } from '$lib/utils';
  import { tablist } from '$lib/actions/tablist';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Tracking's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Board is the index
  // route; Pipeline gets its own path. History and Matches live under Activity.
  const path = $derived(page.url.pathname);
  // Board (index) matches exactly so it is not also active on the child routes.
  const boardActive = $derived(path === '/my/tracking');
  const pipelineActive = $derived(path.startsWith('/my/tracking/pipeline'));
  // The id of the active tab, so the routed panel can point back at it (aria-labelledby).
  const activeTabId = $derived(pipelineActive ? 'tracking-tab-pipeline' : 'tracking-tab-board');

  const tabClass = (active: boolean) =>
    cn(
      'rounded-md px-3 py-1.5 text-sm transition-colors',
      active
        ? 'bg-secondary font-medium text-secondary-foreground'
        : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground',
    );
</script>

<svelte:head>
  <!-- Base title; the child pages override it with their view name. -->
  <title>Tracking — freehire</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <h1 class="text-2xl font-semibold tracking-tight">Tracking</h1>

  <div role="tablist" aria-label="Tracking view" use:tablist={path} class="flex items-center gap-1">
    <a
      role="tab"
      id="tracking-tab-board"
      aria-selected={boardActive}
      aria-controls="tracking-tabpanel"
      href={resolve('/my/tracking')}
      class={tabClass(boardActive)}
    >
      Board
    </a>
    <a
      role="tab"
      id="tracking-tab-pipeline"
      aria-selected={pipelineActive}
      aria-controls="tracking-tabpanel"
      href={resolve('/my/tracking/pipeline')}
      class={tabClass(pipelineActive)}
    >
      Pipeline
    </a>
  </div>

  <div role="tabpanel" id="tracking-tabpanel" aria-labelledby={activeTabId} tabindex="0">
    {@render children()}
  </div>
</div>
