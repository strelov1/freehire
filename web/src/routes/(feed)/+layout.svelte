<script lang="ts">
  import type { Snippet } from 'svelte';
  import { page } from '$app/state';
  import JobsView from '$lib/components/JobsView.svelte';
  import type { LayoutData } from './$types';

  let { data, children }: { data: LayoutData; children: Snippet } = $props();

  // Whether a job is open — drives the rail's selected-card ring and, below the
  // desktop breakpoint, which of {list, pane} is visible. Read from the route's own
  // param (not click state), so a direct link to /jobs/[slug] renders identically to
  // a client-side selection.
  const selectedSlug = $derived(page.params.slug);
</script>

<div class="mx-auto flex w-full max-w-[1600px] gap-6 px-4 py-6 lg:h-[calc(100vh-3.5rem-1px)]">
  <!-- List column: on mobile, hidden once a job is open (the pane takes the full
       width instead); at the desktop breakpoint both columns always show together. -->
  <div
    class={[
      'w-full shrink-0 lg:w-[440px]',
      selectedSlug ? 'hidden lg:block' : 'block',
      'lg:sticky lg:top-[calc(3.5rem+1.5rem)] lg:max-h-[calc(100vh-3.5rem-3rem)] lg:overflow-y-auto',
    ]}
  >
    <JobsView initial={data.initial} layout="stacked" {selectedSlug} />
  </div>

  <!-- Detail pane: on mobile, hidden until a job is open; at the desktop breakpoint
       always shown (the no-selection placeholder from +page.svelte fills it). -->
  <div class={['min-w-0 flex-1', selectedSlug ? 'block' : 'hidden lg:block']}>
    {@render children()}
  </div>
</div>
