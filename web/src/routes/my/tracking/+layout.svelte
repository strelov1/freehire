<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { routeTabClass, tablist } from '$lib/actions/tablist';
  import AccountSetupCard from '$lib/components/AccountSetupCard.svelte';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Tracking's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Board is the index
  // route; Pipeline gets its own path. History and Matches live under Activity.
  const path = $derived(page.url.pathname);
  // Board (index) matches exactly so it is not also active on the child routes.
  const boardActive = $derived(path === '/my/tracking');
  const listActive = $derived(path.startsWith('/my/tracking/list'));
  const pipelineActive = $derived(path.startsWith('/my/tracking/pipeline'));
  const calendarActive = $derived(path.startsWith('/my/tracking/calendar'));
  // The id of the active tab, so the routed panel can point back at it (aria-labelledby).
  const activeTabId = $derived(
    calendarActive
      ? 'tracking-tab-calendar'
      : pipelineActive
        ? 'tracking-tab-pipeline'
        : listActive
          ? 'tracking-tab-list'
          : 'tracking-tab-board',
  );

</script>

<svelte:head>
  <!-- Base title; the child pages override it with their view name. -->
  <title>Tracking — freehire</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <h1 class="text-2xl font-semibold tracking-tight">Tracking</h1>

  <!-- Above the tablist, not inside the panel: what is left to set up belongs to the
       account, not to whichever view of the board happens to be open — and a card inside
       the panel would be re-announced on every tab switch. -->
  <AccountSetupCard />

  <div role="tablist" aria-label="Tracking view" use:tablist={path} class="flex items-center gap-1">
    <a
      role="tab"
      id="tracking-tab-board"
      aria-selected={boardActive}
      aria-controls="tracking-tabpanel"
      href={resolve('/my/tracking')}
      class={routeTabClass(boardActive)}
    >
      Board
    </a>
    <a
      role="tab"
      id="tracking-tab-list"
      aria-selected={listActive}
      aria-controls="tracking-tabpanel"
      href={resolve('/my/tracking/list')}
      class={routeTabClass(listActive)}
    >
      List
    </a>
    <a
      role="tab"
      id="tracking-tab-pipeline"
      aria-selected={pipelineActive}
      aria-controls="tracking-tabpanel"
      href={resolve('/my/tracking/pipeline')}
      class={routeTabClass(pipelineActive)}
    >
      Pipeline
    </a>
    <a
      role="tab"
      id="tracking-tab-calendar"
      aria-selected={calendarActive}
      aria-controls="tracking-tabpanel"
      href={resolve('/my/tracking/calendar')}
      class={routeTabClass(calendarActive)}
    >
      Calendar
    </a>
  </div>

  <div role="tabpanel" id="tracking-tabpanel" aria-labelledby={activeTabId} tabindex="0">
    {@render children()}
  </div>
</div>
