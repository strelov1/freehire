<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { cn } from '$lib/utils';
  import { tablist } from '$lib/actions/tablist';

  let { children }: { children: Snippet } = $props();

  // The account shell (my/+layout) owns the container, auth gate, and noindex;
  // this layout adds only Activity's own sub-navigation. Each view is its own URL
  // so it is linkable, bookmarkable, and survives a reload. Saved is the index
  // route; History/Matches get their own paths.
  const path = $derived(page.url.pathname);
  // Saved (index) matches exactly so it is not also active on the child routes.
  const savedActive = $derived(path === '/my/activity');
  const historyActive = $derived(path.startsWith('/my/activity/history'));
  const matchesActive = $derived(path.startsWith('/my/activity/matches'));
  const hiddenActive = $derived(path.startsWith('/my/activity/hidden'));
  // The id of the active tab, so the routed panel can point back at it (aria-labelledby).
  const activeTabId = $derived(
    historyActive
      ? 'activity-tab-history'
      : matchesActive
        ? 'activity-tab-matches'
        : hiddenActive
          ? 'activity-tab-hidden'
          : 'activity-tab-saved',
  );

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
  <title>Activity — freehire</title>
</svelte:head>

<div class="flex flex-col gap-4">
  <h1 class="text-2xl font-semibold tracking-tight">Activity</h1>

  <div role="tablist" aria-label="Activity view" use:tablist={path} class="flex items-center gap-1">
    <a
      role="tab"
      id="activity-tab-saved"
      aria-selected={savedActive}
      aria-controls="activity-tabpanel"
      href={resolve('/my/activity')}
      class={tabClass(savedActive)}
    >
      Saved
    </a>
    <a
      role="tab"
      id="activity-tab-history"
      aria-selected={historyActive}
      aria-controls="activity-tabpanel"
      href={resolve('/my/activity/history')}
      class={tabClass(historyActive)}
    >
      History
    </a>
    <a
      role="tab"
      id="activity-tab-matches"
      aria-selected={matchesActive}
      aria-controls="activity-tabpanel"
      href={resolve('/my/activity/matches')}
      class={tabClass(matchesActive)}
    >
      Matches
    </a>
    <a
      role="tab"
      id="activity-tab-hidden"
      aria-selected={hiddenActive}
      aria-controls="activity-tabpanel"
      href={resolve('/my/activity/hidden')}
      class={tabClass(hiddenActive)}
    >
      Hidden
    </a>
  </div>

  <div role="tabpanel" id="activity-tabpanel" aria-labelledby={activeTabId} tabindex="0">
    {@render children()}
  </div>
</div>
